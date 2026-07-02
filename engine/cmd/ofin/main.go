// ofin — offline Nigerian legal companion (Week 2 CLI slice).
//
//	ofin ask "question"   retrieve → cited streamed answer (starts servers on demand)
//	ofin retrieve "q"     show fused retrieval results only (debugging/eval)
//	ofin stop             stop the background llama-server processes
//
// Everything runs on 127.0.0.1; zero external network calls at runtime.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"ofin/internal/answer"
	"ofin/internal/llama"
	"ofin/internal/retrieve"
)

const (
	embedPort = 8091
	chatPort  = 8092
	topN      = 6
	maxTokens = 700
	temp      = 0.2
)

func repoRoot() string {
	// The binary lives at engine/bin/ofin or runs via `go run ./cmd/ofin`
	// from engine/ — walk up until we find metadata.json.
	dir, _ := os.Getwd()
	for d := dir; d != "/"; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "metadata.json")); err == nil {
			return d
		}
	}
	return dir
}

func main() {
	root := repoRoot()
	dbPath := flag.String("db", filepath.Join(root, "data/ofin.db"), "retrieval database")
	embedModel := flag.String("embed-model", filepath.Join(root, "models-dev/bge-small-en-v1.5-f16.gguf"), "embedding GGUF")
	chatModel := flag.String("chat-model", filepath.Join(root, "models-dev/Llama-3.2-3B-Instruct-Q4_K_M.gguf"), "chat GGUF")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: ofin [flags] ask|retrieve|stop [question]")
		os.Exit(2)
	}
	cmd := args[0]

	if cmd == "stop" {
		_ = llama.Stop(embedPort)
		_ = llama.Stop(chatPort)
		fmt.Println("stopped llama servers")
		return
	}

	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: ofin %s \"your question\"\n", cmd)
		os.Exit(2)
	}
	question := args[1]

	embed := &llama.Server{Port: embedPort, ModelPath: *embedModel, Embedding: true}
	if err := embed.EnsureRunning(); err != nil {
		fatal(err)
	}

	store, err := retrieve.Open(*dbPath)
	if err != nil {
		fatal(err)
	}
	defer store.Close()

	t0 := time.Now()
	vec, err := embed.Embed(llama.QueryPrefix + question)
	if err != nil {
		fatal(fmt.Errorf("embedding query: %w", err))
	}
	chunks, err := store.Search(vec, question, topN)
	if err != nil {
		fatal(err)
	}
	retrievalMs := time.Since(t0).Milliseconds()

	fmt.Fprintf(os.Stderr, "— retrieved %d sections in %dms:\n", len(chunks), retrievalMs)
	for _, c := range chunks {
		title := ""
		if c.SectionTitle.Valid {
			title = " — " + c.SectionTitle.String
		}
		fmt.Fprintf(os.Stderr, "  %.4f  %s%s\n", c.Score, c.Citation(), title)
	}

	if cmd == "retrieve" {
		return
	}

	chat := &llama.Server{Port: chatPort, ModelPath: *chatModel,
		ExtraArgs: []string{"-c", "8192"}}
	if err := chat.EnsureRunning(); err != nil {
		fatal(err)
	}

	messages := []llama.ChatMessage{
		{Role: "system", Content: answer.SystemPrompt},
		{Role: "user", Content: answer.BuildUserMessage(question, chunks)},
	}
	fmt.Fprintln(os.Stderr, "—")
	t1 := time.Now()
	full, err := chat.ChatStream(messages, maxTokens, temp, func(tok string) {
		fmt.Print(tok)
	})
	if err != nil {
		fatal(err)
	}
	fmt.Println()
	fmt.Fprintf(os.Stderr, "— %d chars in %.1fs\n", len(full), time.Since(t1).Seconds())
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "ofin:", err)
	os.Exit(1)
}
