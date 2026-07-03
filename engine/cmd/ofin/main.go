// ofin — offline Nigerian legal companion.
//
//	ofin ask "question"     retrieve → cited streamed answer
//	ofin retrieve "q"       fused retrieval results only (debugging/eval)
//	ofin serve [--port N]   local web UI (Week 5)
//	ofin stop               stop the background llama-server processes
//
// Everything runs on 127.0.0.1; zero external network calls at runtime.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"ofin/internal/app"
	"ofin/internal/llama"
	"ofin/internal/retrieve"
	"ofin/internal/router"
	"ofin/internal/verify"
	"ofin/internal/web"
)

func repoRoot() string {
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
	cfg := app.DefaultConfig(root)
	flag.StringVar(&cfg.DBPath, "db", cfg.DBPath, "retrieval database")
	flag.StringVar(&cfg.EmbedModel, "embed-model", cfg.EmbedModel, "embedding GGUF")
	flag.StringVar(&cfg.ChatModel, "chat-model", cfg.ChatModel, "chat GGUF")
	draft := flag.Bool("draft", false, "enable speculative decoding (demo machines only, ADR-012; needs the 1B draft GGUF in models-dev/)")
	pidgin := flag.Bool("pidgin", false, "answer in Nigerian Pidgin regardless of question language")
	jsonOut := flag.Bool("json", false, "machine-readable JSON output")
	port := flag.Int("port", 8090, "web UI port (serve)")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: ofin [flags] ask|retrieve|serve|stop [question]")
		os.Exit(2)
	}
	cmd := args[0]

	// Go's flag package stops at the first positional, so `ofin ask -json "q"`
	// would silently ignore -json. Re-parse whatever followed the subcommand.
	if len(args) > 1 {
		if err := flag.CommandLine.Parse(args[1:]); err != nil {
			os.Exit(2)
		}
		args = append([]string{cmd}, flag.Args()...)
	}

	if *draft {
		cfg.DraftModel = root + "/models-dev/Llama-3.2-1B-Instruct-Q4_K_M.gguf"
	}
	a := app.New(cfg)

	if cmd == "stop" {
		a.StopServers()
		fmt.Println("stopped llama servers")
		return
	}

	if cmd == "serve" {
		if len(args) > 1 { // positional port form: ofin serve 8091
			fmt.Sscanf(args[1], "%d", port)
		}
		serve(a, *port)
		return
	}

	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: ofin %s \"your question\"\n", cmd)
		os.Exit(2)
	}
	question := args[1]

	if err := a.Embed.EnsureRunning(); err != nil {
		fatal(err)
	}
	store, err := retrieve.Open(cfg.DBPath)
	if err != nil {
		fatal(err)
	}
	defer store.Close()
	a.Store = store

	vec, err := a.Embed.Embed(llama.QueryPrefix + question)
	if err != nil {
		fatal(fmt.Errorf("embedding: %w", err))
	}

	if cmd == "retrieve" {
		chunks, err := store.Search(vec, question, cfg.TopN)
		if err != nil {
			fatal(err)
		}
		if *jsonOut {
			emitJSON(jsonReport{Question: question, Retrieved: jsonChunks(chunks)})
			return
		}
		fmt.Fprintf(os.Stderr, "— retrieved %d sections\n", len(chunks))
		for _, c := range chunks {
			title := ""
			if c.SectionTitle.Valid {
				title = " — " + c.SectionTitle.String
			}
			fmt.Fprintf(os.Stderr, "  %.4f  %s%s\n", c.Score, c.Citation(), title)
		}
		return
	}

	if err := a.Chat.EnsureRunning(); err != nil {
		fatal(err)
	}

	em := app.Emitter{
		Retrieved: func(chunks []retrieve.Chunk, ms int64) {
			if *jsonOut {
				return
			}
			fmt.Fprintf(os.Stderr, "— retrieved %d sections in %dms:\n", len(chunks), ms)
			for _, c := range chunks {
				title := ""
				if c.SectionTitle.Valid {
					title = " — " + c.SectionTitle.String
				}
				fmt.Fprintf(os.Stderr, "  %.4f  %s%s\n", c.Score, c.Citation(), title)
			}
		},
		Routed: func(kind, summary string) {
			fmt.Fprintf(os.Stderr, "— routed to computation engine (%s): %s\n", kind, summary)
		},
		Computed: func(outcome router.Outcome) {
			if !*jsonOut {
				fmt.Println(outcome.Rendered)
				fmt.Println("\nRECEIPTS")
				fmt.Println("  ✓ computed deterministically by the rules engine — citations shown inline")
			}
		},
		Token: func(tok string) {
			if !*jsonOut {
				fmt.Print(tok)
			}
		},
		AnswerDone: func(text string, wallSec float64) {
			if !*jsonOut {
				fmt.Println()
			}
			fmt.Fprintf(os.Stderr, "— %d chars in %.1fs\n", len(text), wallSec)
		},
		Regenerating: func(failedCount int) {
			if !*jsonOut {
				fmt.Fprintf(os.Stderr, "— %d claim(s) failed verification; regenerating…\n", failedCount)
				fmt.Println("\n--- revised answer ---")
			}
		},
		Receipts: func(results []verify.Result, uncited []string) {
			if *jsonOut {
				return
			}
			printReceipts(results, uncited)
		},
	}

	report, err := a.Ask(question, app.Options{Pidgin: *pidgin}, em)
	if err != nil {
		fatal(err)
	}
	a.Close()

	if *jsonOut {
		emitJSON(jsonReport{
			Question:        report.Question,
			Answer:          report.Answer,
			RetrievalMs:     report.RetrievalMs,
			Regenerated:     report.Regenerated,
			Computation:     report.Computation,
			ComputationJSON: report.ComputationJSON,
			Retrieved:       report.Retrieved,
			Receipts:        report.Receipts,
			Uncited:         report.Uncited,
		})
	}
}

// ---- JSON types (eval-harness contract) ------------------------------------

type jsonChunk = app.RetrievedChunk
type jsonReceipt = app.Receipt

type jsonReport struct {
	Question        string            `json:"question"`
	Answer          string            `json:"answer,omitempty"`
	RetrievalMs     int64             `json:"retrieval_ms"`
	Regenerated     bool              `json:"regenerated,omitempty"`
	Computation     string            `json:"computation,omitempty"`
	ComputationJSON json.RawMessage   `json:"computation_result,omitempty"`
	Retrieved       []jsonChunk       `json:"retrieved"`
	Receipts        []jsonReceipt     `json:"receipts,omitempty"`
	Uncited         []string          `json:"uncited,omitempty"`
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

func emitJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// ---- CLI receipts output ---------------------------------------------------

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

// ---- serve stub (replaced by web package in the next step) -----------------

func serve(a *app.App, port int) {
	if err := web.Serve(port, a); err != nil {
		fatal(err)
	}
}
