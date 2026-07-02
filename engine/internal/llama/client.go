// Package llama manages local llama.cpp server processes and speaks their
// OpenAI-compatible HTTP API. Everything is 127.0.0.1 — the runtime makes no
// external network calls.
package llama

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// BGE embedding models require this prefix on QUERIES only (ADR-008).
const QueryPrefix = "Represent this sentence for searching relevant passages: "

type Server struct {
	Port      int
	ModelPath string
	Embedding bool
	ExtraArgs []string
}

func (s *Server) baseURL() string { return fmt.Sprintf("http://127.0.0.1:%d", s.Port) }

func (s *Server) Healthy() bool {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(s.baseURL() + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// EnsureRunning starts a llama-server for this model if one is not already
// healthy on the port, and waits for it to come up. The child is detached:
// it stays alive across CLI invocations so subsequent asks skip model load.
func (s *Server) EnsureRunning() error {
	if s.Healthy() {
		return nil
	}
	args := []string{
		"-m", s.ModelPath,
		"--port", fmt.Sprint(s.Port),
		"--host", "127.0.0.1",
	}
	if s.Embedding {
		args = append(args, "--embedding", "-c", "512", "-ub", "512")
	}
	args = append(args, s.ExtraArgs...)
	cmd := exec.Command("llama-server", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting llama-server: %w", err)
	}
	go cmd.Wait() // reap if it dies; health checks are the real signal

	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if s.Healthy() {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("llama-server on port %d not healthy after 90s", s.Port)
}

func Stop(port int) error {
	// llama-server has no shutdown endpoint; match on the port argument.
	return exec.Command("pkill", "-f", fmt.Sprintf("llama-server.*--port %d", port)).Run()
}

func (s *Server) Embed(text string) ([]float32, error) {
	payload, _ := json.Marshal(map[string]any{"input": []string{text}})
	resp, err := http.Post(s.baseURL()+"/v1/embeddings", "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return out.Data[0].Embedding, nil
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatStream sends a chat completion request and calls onToken for each
// content delta. Returns the full assembled response text.
func (s *Server) ChatStream(messages []ChatMessage, maxTokens int, temp float64, onToken func(string)) (string, error) {
	payload, _ := json.Marshal(map[string]any{
		"messages":    messages,
		"max_tokens":  maxTokens,
		"temperature": temp,
		"stream":      true,
	})
	resp, err := http.Post(s.baseURL()+"/v1/chat/completions", "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("chat request failed (%d): %s", resp.StatusCode, body)
	}

	var full strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		for _, c := range chunk.Choices {
			if c.Delta.Content != "" {
				full.WriteString(c.Delta.Content)
				if onToken != nil {
					onToken(c.Delta.Content)
				}
			}
		}
	}
	return full.String(), scanner.Err()
}
