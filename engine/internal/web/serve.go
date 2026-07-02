// Package web serves the local web UI — a single-page app that streams
// Òfin answers via SSE. No build toolchain: go:embed serves vanilla
// HTML/CSS/JS. Listeners are bound to 127.0.0.1 only (offline guarantee).
package web

import (
	"compress/gzip"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"ofin/internal/app"
	"ofin/internal/retrieve"
	"ofin/internal/router"
	"ofin/internal/verify"
)

//go:embed index.html
var staticFS embed.FS

// Handler returns an http.Handler for the Òfin UI.
func Handler(a *app.App) http.Handler {
	mux := http.NewServeMux()

	index, _ := fs.Sub(staticFS, ".")
	mux.Handle("/", gzipHandler(http.FileServer(http.FS(index))))

	mux.HandleFunc("/api/ask", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct{ Question string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Question == "" {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		handleAsk(w, a, body.Question)
	})

	return mux
}

type sseWriter interface {
	http.ResponseWriter
	http.Flusher
}

func handleAsk(w http.ResponseWriter, a *app.App, question string) {
	f, ok := w.(sseWriter)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	send := func(v map[string]any) {
		data, err := json.Marshal(v)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		f.Flush()
	}

	em := app.Emitter{
		Retrieved: func(chunks []retrieve.Chunk, ms int64) {
			items := make([]map[string]any, 0, len(chunks))
			for _, c := range chunks {
				title := ""
				if c.SectionTitle.Valid {
					title = c.SectionTitle.String
				}
				items = append(items, map[string]any{
					"act": c.ActShort, "section": c.SectionID, "title": title,
					"score": c.Score, "text": truncate(c.Text, 4000),
				})
			}
			send(map[string]any{"type": "retrieved", "chunks": items, "ms": ms})
		},
		Routed: func(kind, summary string) {
			send(map[string]any{"type": "routed", "kind": kind, "summary": summary})
		},
		Computed: func(outcome router.Outcome) {
			send(map[string]any{"type": "computed", "kind": outcome.Kind,
				"rendered": outcome.Rendered, "html": app.ComputationHTML(outcome)})
		},
		Token: func(tok string) {
			send(map[string]any{"type": "token", "text": tok})
		},
		AnswerDone: func(text string, wallSec float64) {
			send(map[string]any{"type": "answer_done", "text": text, "wall_sec": wallSec})
		},
		Regenerating: func(failedCount int) {
			send(map[string]any{"type": "regenerating", "failed_count": failedCount})
		},
		Receipts: func(results []verify.Result, uncited []string) {
			receipts := make([]app.Receipt, 0, len(results))
			for _, r := range results {
				receipts = append(receipts, app.Receipt{
					Verdict: r.Verdict.String(), SourceRef: r.SourceRef,
					Claim: r.Claim.Text, Reasons: r.Reasons, Similarity: r.Similarity,
					SourceText: r.SourceText,
				})
			}
			send(map[string]any{"type": "receipts", "receipts": receipts, "uncited": uncited})
		},
	}

	report, err := a.Ask(question, em)
	if err != nil {
		send(map[string]any{"type": "error", "message": err.Error()})
		return
	}
	_ = report
	send(map[string]any{"type": "done"})
}

// Serve starts the HTTP server on the given port (127.0.0.1 only).
func Serve(port int, a *app.App) error {
	if err := a.EnsureReady(); err != nil {
		return fmt.Errorf("starting servers: %w", err)
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	log.Printf("Òfin ready at http://%s", addr)
	return http.ListenAndServe(addr, Handler(a))
}

// gzipHandler wraps an http.Handler with gzip compression for text/* and
// application/* content types.
func gzipHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		gzw := &gzipResponseWriter{ResponseWriter: w, Writer: gz}
		next.ServeHTTP(gzw, r)
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

type gzipResponseWriter struct {
	http.ResponseWriter
	Writer *gzip.Writer
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	return g.Writer.Write(b)
}
