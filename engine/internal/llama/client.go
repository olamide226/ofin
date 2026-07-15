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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// BGE embedding models require this prefix on QUERIES only (ADR-008).
const QueryPrefix = "Represent this sentence for searching relevant passages: "

type Server struct {
	Port       int
	ModelPath  string
	DraftModel string // optional: speculative decoding draft GGUF
	Embedding  bool
	ExtraArgs  []string
	cmd        *exec.Cmd // the running child process (nil if not started by us)
	pidFile    string    // path to PID file for cross-invocation stopping
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

// llamaServerPath locates the llama-server binary. It prefers a copy bundled
// next to the ofin executable (packaged app layout: <exe-dir>/llama/ or
// <exe-dir>/), then falls back to the name so PATH resolution applies in
// development. Returning an absolute path avoids Go's refusal to exec a
// binary found via a relative PATH entry (ErrDot).
func llamaServerPath() string {
	name := "llama-server"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, cand := range []string{
			filepath.Join(dir, "llama", name), // packaged: Resources/llama/llama-server
			filepath.Join(dir, name),          // flat: alongside the binary
		} {
			if info, err := os.Stat(cand); err == nil && !info.IsDir() {
				return cand
			}
		}
	}
	return name // dev: resolve via PATH
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
	if s.DraftModel != "" {
		// A missing draft GGUF must never take the chat server down with it
		// (llama-server exits at startup if --model-draft doesn't resolve).
		if _, err := os.Stat(s.DraftModel); err != nil {
			fmt.Fprintf(os.Stderr, "ofin: draft model %s not found; continuing without speculative decoding\n", s.DraftModel)
		} else {
			args = append(args, "--model-draft", s.DraftModel)
		}
	}
	args = append(args, s.ExtraArgs...)
	s.cmd = exec.Command(llamaServerPath(), args...)
	// Capture stderr so a startup failure (missing shared library, bad model
	// file, port conflict, ...) is diagnosable instead of a bare "not healthy"
	// timeout — llama-server logs its fatal error there before exiting.
	// Guarded by a mutex: exec.Cmd copies into this writer from its own
	// goroutine, concurrently with reads below while we're still polling.
	stderr := &syncBuffer{}
	s.cmd.Stdout = nil
	s.cmd.Stderr = stderr
	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("starting llama-server: %w", err)
	}
	// Write PID file for cross-invocation stopping (ofin stop).
	if s.pidFile != "" {
		os.WriteFile(s.pidFile, []byte(fmt.Sprint(s.cmd.Process.Pid)), 0644)
	}
	exited := make(chan error, 1)
	go func() { exited <- s.cmd.Wait() }()

	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if s.Healthy() {
			return nil
		}
		select {
		case <-exited:
			return fmt.Errorf("llama-server on port %d exited during startup: %s", s.Port, lastLines(stderr.String(), 5))
		default:
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("llama-server on port %d not healthy after 90s: %s", s.Port, lastLines(stderr.String(), 5))
}

// lastLines returns the last n non-empty lines of s, for compact error
// messages that surface llama-server's own fatal log line to the user.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}

// syncBuffer is a bytes.Buffer safe for concurrent Write (from exec.Cmd's
// output-copying goroutine) and String (from the polling loop) calls.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// pidFilePath returns the expected PID file path for a given port.
func pidFilePath(port int) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("ofin-llama-%d.pid", port))
}

// Stop kills a llama-server on the given port. It tries the PID file first
// (written by a previous invocation), then falls back to pkill/taskkill.
func Stop(port int) error {
	// 1. Try PID file first — most reliable cross-platform.
	pidFile := pidFilePath(port)
	if data, err := os.ReadFile(pidFile); err == nil {
		var pid int
		if _, scanErr := fmt.Sscanf(string(data), "%d", &pid); scanErr == nil && pid > 0 {
			proc, _ := os.FindProcess(pid)
			if proc != nil {
				proc.Kill()
			}
		}
		os.Remove(pidFile)
	}
	// 2. Fallback: pkill/taskkill by port pattern.
	if runtime.GOOS == "windows" {
		return exec.Command("taskkill", "/F", "/FI",
			fmt.Sprintf("COMMAND eq llama-server.exe")).Run()
	}
	return exec.Command("pkill", "-f",
		fmt.Sprintf("llama-server.*--port %d", port)).Run()
}

// EmbedBatch embeds several texts in one request — llama-server accepts an
// array input, saving a round-trip per text. The verifier prefetches every
// claim and cited source this way (one call instead of ~2 per claim).
func (s *Server) EmbedBatch(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	payload, _ := json.Marshal(map[string]any{"input": texts})
	resp, err := http.Post(s.baseURL()+"/v1/embeddings", "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Data) != len(texts) {
		return nil, fmt.Errorf("embedding batch: sent %d texts, got %d vectors", len(texts), len(out.Data))
	}
	vecs := make([][]float32, len(texts))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(texts) {
			return nil, fmt.Errorf("embedding batch: index %d out of range", d.Index)
		}
		vecs[d.Index] = d.Embedding
	}
	return vecs, nil
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
