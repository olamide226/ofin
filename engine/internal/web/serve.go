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
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"ofin/internal/app"
	"ofin/internal/download"
	"ofin/internal/retrieve"
	"ofin/internal/router"
	"ofin/internal/verify"
)

//go:embed index.html
var staticFS embed.FS

//go:embed setup.html
var setupHTML []byte

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
		var body struct {
			Question string
			Pidgin   bool
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Question == "" {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		handleAsk(w, a, body.Question, app.Options{Pidgin: body.Pidgin})
	})

	return mux
}

type sseWriter interface {
	http.ResponseWriter
	http.Flusher
}

func handleAsk(w http.ResponseWriter, a *app.App, question string, opts app.Options) {
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

	report, err := a.Ask(question, opts, em)
	if err != nil {
		send(map[string]any{"type": "error", "message": err.Error()})
		return
	}
	_ = report
	send(map[string]any{"type": "done"})
}

// stateServer serves either the first-time setup page (while the model
// downloads) or the full app once the model is present and llama-servers
// are healthy. A single atomic flag flips between the two — the same binary
// handles both, so `ofin serve` works identically in development (model
// already present, setup never shown) and in a packaged install (model
// missing, setup page downloads it with live progress).
type stateServer struct {
	app        *app.App
	appHandler http.Handler
	ready      atomic.Bool

	mu   sync.Mutex
	prog setupProgress
}

type setupProgress struct {
	Downloaded int64  `json:"downloaded"`
	Total      int64  `json:"total"`
	Ready      bool   `json:"ready"`
	Error      string `json:"error,omitempty"`
}

func (s *stateServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.ready.Load() {
		s.appHandler.ServeHTTP(w, r)
		return
	}
	// Setup mode.
	switch r.URL.Path {
	case "/api/setup-status":
		s.mu.Lock()
		p := s.prog
		s.mu.Unlock()
		p.Ready = s.ready.Load()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	default:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(setupHTML)
	}
}

// runSetup downloads the model with live progress, then starts the
// llama-servers and flips the ready flag. Runs in a background goroutine.
func (s *stateServer) runSetup() {
	_, err := download.Model(s.app.Config.ChatModel, func(dl, total int64) {
		s.mu.Lock()
		s.prog.Downloaded, s.prog.Total = dl, total
		s.mu.Unlock()
	})
	if err != nil {
		s.mu.Lock()
		s.prog.Error = "Download failed: " + err.Error() + " — check your internet connection and restart Òfin."
		s.mu.Unlock()
		log.Printf("model download failed: %v", err)
		return
	}
	if err := s.app.EnsureReady(); err != nil {
		s.mu.Lock()
		s.prog.Error = "Model downloaded but the engine failed to start: " + err.Error()
		s.mu.Unlock()
		log.Printf("EnsureReady after download failed: %v", err)
		return
	}
	s.ready.Store(true)
	log.Printf("Model ready — Òfin is now live.")
}

// Serve starts the HTTP server on the given port (127.0.0.1 only).
// If the chat model is missing, it serves a setup page and downloads the
// model in the background before starting the engine.
func Serve(port int, a *app.App) error {
	s := &stateServer{app: a, appHandler: Handler(a)}
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	if _, err := os.Stat(a.Config.ChatModel); err == nil {
		// Model present — normal startup (blocks until servers are healthy).
		if err := a.EnsureReady(); err != nil {
			return fmt.Errorf("starting servers: %w", err)
		}
		s.ready.Store(true)
		log.Printf("Òfin ready at http://%s", addr)
	} else {
		// Model missing — first-time setup. Serve the progress page and
		// download in the background; the page reloads into the app when done.
		log.Printf("First-time setup — open http://%s to watch the download", addr)
		go s.runSetup()
	}
	return http.ListenAndServe(addr, s)
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
