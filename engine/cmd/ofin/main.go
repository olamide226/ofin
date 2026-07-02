// ofin — offline Nigerian legal companion (Week 2 CLI slice).
//
//	ofin ask "question"   retrieve → cited streamed answer (starts servers on demand)
//	ofin retrieve "q"     show fused retrieval results only (debugging/eval)
//	ofin stop             stop the background llama-server processes
//
// Everything runs on 127.0.0.1; zero external network calls at runtime.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"ofin/internal/answer"
	"ofin/internal/llama"
	"ofin/internal/retrieve"
	"ofin/internal/router"
	"ofin/internal/verify"
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
	jsonOut := flag.Bool("json", false, "machine-readable JSON output (for the eval harness)")
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

	if !*jsonOut {
		fmt.Fprintf(os.Stderr, "— retrieved %d sections in %dms:\n", len(chunks), retrievalMs)
		for _, c := range chunks {
			title := ""
			if c.SectionTitle.Valid {
				title = " — " + c.SectionTitle.String
			}
			fmt.Fprintf(os.Stderr, "  %.4f  %s%s\n", c.Score, c.Citation(), title)
		}
	}

	if cmd == "retrieve" {
		if *jsonOut {
			emitJSON(jsonReport{Question: question, RetrievalMs: retrievalMs,
				Retrieved: jsonChunks(chunks)})
		}
		return
	}

	chat := &llama.Server{Port: chatPort, ModelPath: *chatModel,
		ExtraArgs: []string{"-c", "8192"}}
	if err := chat.EnsureRunning(); err != nil {
		fatal(err)
	}

	// Intent router: one silent extraction call decides lookup vs
	// computation. Computation answers render DETERMINISTICALLY — no LLM
	// ever touches the figures (a 3B model recomputes numbers it was told
	// to transcribe: observed live inventing ₦35,000 against a computed
	// ₦63,500). Model-polished phrasing around the numbers is a Week-6
	// layer; the numbers themselves never pass through the model.
	if extRaw, err := chat.ChatStream([]llama.ChatMessage{
		{Role: "system", Content: router.ExtractionPrompt},
		{Role: "user", Content: question},
	}, 250, 0, nil); err == nil {
		if p, err := router.ParseParams(extRaw); err == nil {
			if outcome, ok := router.Computation(p, question, time.Now()); ok {
				fmt.Fprintf(os.Stderr, "— routed to computation engine (%s): %s\n",
					outcome.Kind, outcome.Summary)
				if *jsonOut {
					emitJSON(jsonReport{Question: question, RetrievalMs: retrievalMs,
						Answer: outcome.Rendered, Computation: outcome.Kind,
						ComputationJSON: json.RawMessage(outcome.JSON),
						Retrieved:       jsonChunks(chunks)})
					return
				}
				fmt.Println(outcome.Rendered)
				fmt.Println("\nRECEIPTS")
				fmt.Println("  ✓ computed deterministically by the rules engine — citations shown inline")
				return
			}
		}
	}

	messages := []llama.ChatMessage{
		{Role: "system", Content: answer.SystemPrompt},
		{Role: "user", Content: answer.BuildUserMessage(question, chunks)},
	}
	stream := func(tok string) {
		if !*jsonOut {
			fmt.Print(tok)
		}
	}
	fmt.Fprintln(os.Stderr, "—")
	t1 := time.Now()
	full, err := chat.ChatStream(messages, maxTokens, temp, stream)
	if err != nil {
		fatal(err)
	}
	if !*jsonOut {
		fmt.Println()
	}
	fmt.Fprintf(os.Stderr, "— %d chars in %.1fs\n", len(full), time.Since(t1).Seconds())

	verifier := &verify.Verifier{Corpus: store, Embed: embed, Resolve: store.ResolveAct}
	results, uncited := verifier.VerifyAnswer(question, full)
	regenerated := false

	// One constrained regeneration pass when any claim failed: re-prompt
	// with the failures spelled out and the correct statutory text injected.
	if failed := failedResults(results); len(failed) > 0 {
		regenerated = true
		fmt.Fprintf(os.Stderr, "— %d claim(s) failed verification; regenerating…\n", len(failed))
		messages = append(messages,
			llama.ChatMessage{Role: "assistant", Content: full},
			llama.ChatMessage{Role: "user", Content: answer.BuildCorrectionMessage(failed)},
		)
		if !*jsonOut {
			fmt.Println("\n--- revised answer ---")
		}
		full, err = chat.ChatStream(messages, maxTokens, temp, stream)
		if err != nil {
			fatal(err)
		}
		if !*jsonOut {
			fmt.Println()
		}
		results, uncited = verifier.VerifyAnswer(question, full)
	}

	if *jsonOut {
		emitJSON(jsonReport{
			Question: question, Answer: full, RetrievalMs: retrievalMs,
			Regenerated: regenerated, Retrieved: jsonChunks(chunks),
			Receipts: jsonReceipts(results), Uncited: uncited,
		})
		return
	}
	printReceipts(results, uncited)
}

type jsonChunk struct {
	Act     string  `json:"act"`
	Section string  `json:"section"`
	Score   float64 `json:"score"`
}

type jsonReceipt struct {
	Verdict    string   `json:"verdict"`
	SourceRef  string   `json:"source_ref"`
	Claim      string   `json:"claim"`
	Reasons    []string `json:"reasons,omitempty"`
	Similarity float64  `json:"similarity"`
}

type jsonReport struct {
	Question        string          `json:"question"`
	Answer          string          `json:"answer,omitempty"`
	RetrievalMs     int64           `json:"retrieval_ms"`
	Regenerated     bool            `json:"regenerated,omitempty"`
	Computation     string          `json:"computation,omitempty"`
	ComputationJSON json.RawMessage `json:"computation_result,omitempty"`
	Retrieved       []jsonChunk     `json:"retrieved"`
	Receipts        []jsonReceipt   `json:"receipts,omitempty"`
	Uncited         []string        `json:"uncited,omitempty"`
}

func jsonChunks(chunks []retrieve.Chunk) []jsonChunk {
	out := make([]jsonChunk, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, jsonChunk{Act: c.ActShort, Section: c.SectionID, Score: c.Score})
	}
	return out
}

func jsonReceipts(results []verify.Result) []jsonReceipt {
	out := make([]jsonReceipt, 0, len(results))
	for _, r := range results {
		out = append(out, jsonReceipt{
			Verdict: r.Verdict.String(), SourceRef: r.SourceRef,
			Claim: r.Claim.Text, Reasons: r.Reasons, Similarity: r.Similarity,
		})
	}
	return out
}

func emitJSON(r jsonReport) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		fatal(err)
	}
}

func failedResults(results []verify.Result) []verify.Result {
	var out []verify.Result
	for _, r := range results {
		if r.Verdict == verify.Failed {
			out = append(out, r)
		}
	}
	return out
}

func printReceipts(results []verify.Result, uncited []string) {
	fmt.Println("\nRECEIPTS")
	marks := map[verify.Verdict]string{
		verify.Verified: "✓", verify.Flagged: "⚠", verify.Failed: "✗",
	}
	for _, r := range results {
		ref := r.SourceRef
		if ref == "" && len(r.Claim.Citations) > 0 {
			ref = r.Claim.Citations[0].Raw
		}
		fmt.Printf("  %s %s  %s\n", marks[r.Verdict], ref, snippet(r.Claim.Text, 90))
		for _, reason := range r.Reasons {
			fmt.Printf("      %s\n", reason)
		}
	}
	for _, u := range uncited {
		fmt.Printf("  · uncited (general guidance, not verified against statute): %s\n", snippet(u, 90))
	}
}

func snippet(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "ofin:", err)
	os.Exit(1)
}
