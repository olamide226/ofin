// Package app is Òfin's central ask-orchestration. The CLI and the web UI
// both call App.Ask; they differ only in how they consume events.
package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ofin/internal/answer"
	"ofin/internal/llama"
	"ofin/internal/retrieve"
	"ofin/internal/router"
	"ofin/internal/rules"
	"ofin/internal/verify"
)

// Config holds the paths and ports for a session.
type Config struct {
	DBPath      string
	EmbedModel  string
	ChatModel   string
	DraftModel  string // optional: speculative decoding draft GGUF
	EmbedPort   int
	ChatPort    int
	TopN        int
	MaxTokens   int
	Temp        float64
	ChatCtxSize int
}

// DefaultConfig returns the config used by the CLI and development.
func DefaultConfig(root string) Config {
	return Config{
		DBPath:     root + "/data/ofin.db",
		EmbedModel: root + "/models-dev/bge-small-en-v1.5-f16.gguf",
		ChatModel:  root + "/models-dev/Llama-3.2-3B-Instruct-Q4_K_M.gguf",
		// Speculative decoding is OFF by default (ADR-012): the draft GGUF
		// is not shipped by download_model.sh, and a default path here made
		// the chat server crash on fresh clones. Opt in with `ofin -draft`.
		DraftModel:  "",
		EmbedPort:   8091,
		ChatPort:    8092,
		TopN:        6,
		MaxTokens:   700,
		Temp:        0.2,
		ChatCtxSize: 6144, // trimmed from 8192 after prompt diet
	}
}

// Report is the structured result of an Ask, used for JSON output
// (eval harness) and for rendering receipts in the UI.
type Report struct {
	Question        string          `json:"question"`
	Answer          string          `json:"answer,omitempty"`
	RetrievalMs     int64           `json:"retrieval_ms"`
	Regenerated     bool            `json:"regenerated,omitempty"`
	Computation     string          `json:"computation,omitempty"` // kind
	ComputationJSON json.RawMessage `json:"computation_result,omitempty"`
	ComputationHTML string          `json:"computation_html,omitempty"` // pre-rendered breakdown
	Retrieved       []RetrievedChunk `json:"retrieved"`
	Receipts        []Receipt        `json:"receipts,omitempty"`
	Uncited         []string         `json:"uncited,omitempty"`
	SourceText      map[string]string `json:"source_text,omitempty"` // citation-ref -> statutory text
}

// RetrievedChunk is a retrieved section, for JSON output and UI preview.
type RetrievedChunk struct {
	Act     string  `json:"act"`
	Section string  `json:"section"`
	Title   string  `json:"title,omitempty"`
	Score   float64 `json:"score"`
	Text    string  `json:"-"` // full text is for SourceText map, not serialized inline
}

// Receipt is one verified-or-flagged claim outcome.
type Receipt struct {
	Verdict    string   `json:"verdict"`
	SourceRef  string   `json:"source_ref"`
	Claim      string   `json:"claim"`
	Reasons    []string `json:"reasons,omitempty"`
	Similarity float64  `json:"similarity"`
	SourceText string   `json:"source_text,omitempty"`
}

// Emitter is the callback interface for streaming Ask events. Every method
// is optional — the nil receiver is safe for all callbacks (a no-op).
type Emitter struct {
	Retrieved  func(chunks []retrieve.Chunk, ms int64)
	Routed     func(kind, summary string)
	Computed   func(outcome router.Outcome)
	Token      func(s string)
	AnswerDone func(text string, wallSec float64)
	Receipts   func(results []verify.Result, uncited []string)
	Regenerating func(failedCount int)
}

// App is the ask-orchestration engine. One per session; the methods it holds
// are the pieces the CLI and web server wire up — embedder, retriever,
// generator, verifier — all connected.
type App struct {
	Config Config
	Embed  *llama.Server
	Chat   *llama.Server
	Store  *retrieve.Store
}

// New creates an App from a Config. Caller must call EnsureReady before Ask.
func New(cfg Config) *App {
	return &App{
		Config: cfg,
		Embed: &llama.Server{
			Port: cfg.EmbedPort, ModelPath: cfg.EmbedModel,
			Embedding: true,
		},
		Chat: &llama.Server{
			Port: cfg.ChatPort, ModelPath: cfg.ChatModel,
			DraftModel: cfg.DraftModel,
			// KV-cache quantization reduces memory; flash attention and
			// thread tuning improve throughput. All are dev-UX only per
			// ADR-003 (the audit profiles the raw GGUF).
			ExtraArgs: []string{"-c", fmt.Sprint(cfg.ChatCtxSize),
				"-ctk", "q8_0", "-ctv", "q8_0"},
		},
	}
}

// EnsureReady starts both llama-servers if they are not already healthy.
func (a *App) EnsureReady() error {
	if err := a.Embed.EnsureRunning(); err != nil {
		return err
	}
	if a.Store == nil {
		s, err := retrieve.Open(a.Config.DBPath)
		if err != nil {
			return fmt.Errorf("opening %s: %w", a.Config.DBPath, err)
		}
		a.Store = s
	}
	return a.Chat.EnsureRunning()
}

// Close releases the store. llama-server processes stay alive across
// invocations; use StopServers() to kill them.
func (a *App) Close() {
	if a.Store != nil {
		a.Store.Close()
		a.Store = nil
	}
}

// StopServers kills both background servers.
func (a *App) StopServers() {
	_ = llama.Stop(a.Config.EmbedPort)
	_ = llama.Stop(a.Config.ChatPort)
}

// Ask runs the full retrieval → route → compute/generate → verify pipeline.
// Every stage calls the matching Emitter callback so the CLI can log to
// stderr and the web UI can push SSE events.
func (a *App) Ask(question string, em Emitter) (*Report, error) {
	report := &Report{Question: question}

	t0 := time.Now()
	vec, err := a.Embed.Embed(llama.QueryPrefix + question)
	if err != nil {
		return nil, fmt.Errorf("embedding: %w", err)
	}
	chunks, err := a.Store.Search(vec, question, a.Config.TopN)
	if err != nil {
		return nil, fmt.Errorf("retrieval: %w", err)
	}
	report.RetrievalMs = time.Since(t0).Milliseconds()

	report.Retrieved = make([]RetrievedChunk, len(chunks))
	report.SourceText = make(map[string]string, len(chunks))
	for i, c := range chunks {
		title := ""
		if c.SectionTitle.Valid {
			title = c.SectionTitle.String
		}
		report.Retrieved[i] = RetrievedChunk{
			Act: c.ActShort, Section: c.SectionID, Title: title,
			Score: c.Score, Text: c.Text,
		}
		report.SourceText[c.Citation()] = c.Text
	}
	if em.Retrieved != nil {
		em.Retrieved(chunks, report.RetrievalMs)
	}

	// Intent router: one silent extraction → deterministic computation.
	if extRaw, err := a.Chat.ChatStream([]llama.ChatMessage{
		{Role: "system", Content: router.ExtractionPrompt},
		{Role: "user", Content: question},
	}, 250, 0, nil); err == nil {
		if p, err := router.ParseParams(extRaw); err == nil {
			if outcome, ok := router.Computation(p, question, time.Now()); ok {
				report.Computation = outcome.Kind
				report.ComputationJSON = json.RawMessage(outcome.JSON)
				report.Answer = outcome.Rendered
				report.ComputationHTML = ComputationHTML(outcome)
				if em.Routed != nil {
					em.Routed(outcome.Kind, outcome.Summary)
				}
				if em.Computed != nil {
					em.Computed(outcome)
				}
				return report, nil
			}
		}
	}

	// Lookup path: generate a cited answer, then verify.
	messages := []llama.ChatMessage{
		{Role: "system", Content: answer.SystemPrompt},
		{Role: "user", Content: answer.BuildUserMessage(question, chunks, a.Config.TopN)},
	}
	var full string
	onToken := em.Token
	if onToken == nil {
		onToken = func(string) {}
	}
	t1 := time.Now()
	full, err = a.Chat.ChatStream(messages, a.Config.MaxTokens, a.Config.Temp, onToken)
	if err != nil {
		return nil, fmt.Errorf("generation: %w", err)
	}
	report.Answer = full
	if em.AnswerDone != nil {
		em.AnswerDone(full, time.Since(t1).Seconds())
	}

	verifier := &verify.Verifier{Corpus: a.Store, Embed: a.Embed, Resolve: a.Store.ResolveAct}
	results, uncited := verifier.VerifyAnswer(question, full)

	// One constrained regeneration pass when any claim failed.
	if failed := failedResults(results); len(failed) > 0 {
		report.Regenerated = true
		if em.Regenerating != nil {
			em.Regenerating(len(failed))
		}
		messages = append(messages,
			llama.ChatMessage{Role: "assistant", Content: full},
			llama.ChatMessage{Role: "user", Content: answer.BuildCorrectionMessage(failed)},
		)
		full, err = a.Chat.ChatStream(messages, a.Config.MaxTokens, a.Config.Temp, onToken)
		if err != nil {
			return nil, fmt.Errorf("regeneration: %w", err)
		}
		report.Answer = full
		results, uncited = verifier.VerifyAnswer(question, full)
	}

	report.Receipts = makeReceipts(results)
	report.Uncited = uncited
	if em.Receipts != nil {
		em.Receipts(results, uncited)
	}
	return report, nil
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

func makeReceipts(results []verify.Result) []Receipt {
	out := make([]Receipt, 0, len(results))
	for _, r := range results {
		out = append(out, Receipt{
			Verdict: r.Verdict.String(), SourceRef: r.SourceRef,
			Claim: r.Claim.Text, Reasons: r.Reasons, Similarity: r.Similarity,
			SourceText: r.SourceText,
		})
	}
	return out
}

// ComputationHTML renders a computation outcome as an HTML breakdown
// (PAYE band table, notice card). Used by the JSON report and the web UI.
func ComputationHTML(outcome router.Outcome) string {
	switch outcome.Kind {
	case "paye":
		return renderPAYETable(outcome)
	case "termination_notice":
		return renderNoticeCard(outcome)
	}
	return ""
}

func renderPAYETable(outcome router.Outcome) string {
	var res rules.PAYEResult
	_ = json.Unmarshal([]byte(outcome.JSON), &res)
	var b strings.Builder
	b.WriteString(`<div class="computation paye">`)
	b.WriteString(fmt.Sprintf(`<p class="outcome">Your PAYE is <strong>₦%s/month</strong> (₦%s/year, effective rate %.1f%%)</p>`,
		formatNaira(res.MonthlyTax), formatNaira(res.AnnualTax), res.EffectiveRate*100))
	b.WriteString(`<table><thead><tr><th>Band</th><th>Amount (₦)</th><th>Citation</th></tr></thead><tbody>`)
	for _, line := range res.Bands {
		b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td class="num">%s</td><td>%s</td></tr>`,
			line.Label, formatNaira(line.Amount), line.Citation))
	}
	b.WriteString(`</tbody></table>`)
	if res.RentRelief > 0 {
		b.WriteString(fmt.Sprintf(`<p class="note">Rent relief: −₦%s (20%% of rent, capped ₦500,000) [Nigeria Tax Act 2025]</p>`, formatNaira(res.RentRelief)))
	}
	b.WriteString(fmt.Sprintf(`<p class="basis">%s. Computed deterministically.</p>`, res.AsAt))
	b.WriteString(`</div>`)
	return b.String()
}

func renderNoticeCard(outcome router.Outcome) string {
	var res rules.NoticeResult
	_ = json.Unmarshal([]byte(outcome.JSON), &res)
	var b strings.Builder
	b.WriteString(`<div class="computation notice">`)
	b.WriteString(fmt.Sprintf(`<p class="outcome">Minimum notice: <strong>%s</strong> %s (%.1f years)</p>`,
		res.Notice, res.Citation, res.TenureMonths/12))
	b.WriteString(`<ul>`)
	for _, n := range res.Notes {
		b.WriteString(fmt.Sprintf(`<li>%s %s</li>`, n.Label, n.Citation))
	}
	b.WriteString(`</ul>`)
	b.WriteString(fmt.Sprintf(`<p class="basis">%s. Computed deterministically.</p>`, res.AsAt))
	b.WriteString(`</div>`)
	return b.String()
}

func formatNaira(v float64) string {
	s := fmt.Sprintf("%.0f", v)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}
