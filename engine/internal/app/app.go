// Package app is Òfin's central ask-orchestration. The CLI and the web UI
// both call App.Ask; they differ only in how they consume events.
package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"ofin/internal/answer"
	"ofin/internal/llama"
	"ofin/internal/retrieve"
	"ofin/internal/router"
	"regexp"
	"sort"
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
	TopN        int         // fused sources retrieved per question
	Pack        answer.Pack // how those sources are packed into the prompt (prefill lever)
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
		EmbedPort: 8091,
		ChatPort:  8092,
		// 8 fused sources, full text for the top 4 only: eval 2026-07-03
		// found genuine hits parked at ranks 7-8 (L03, E08, XD02) after the
		// corpus grew to 678 chunks. Widening the window while narrowing the
		// full-text tier costs LESS prefill than the old 6-full layout.
		TopN:        8,
		Pack:        answer.DefaultPack,
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
	Config      Config
	Embed       *llama.Server
	Chat        *llama.Server
	Store       *retrieve.Store
	verifyCache *verify.Cache // embeddings memo shared across asks
}

// New creates an App from a Config. Caller must call EnsureReady before Ask.
func New(cfg Config) *App {
	return &App{
		Config:      cfg,
		verifyCache: verify.NewCache(),
		Embed: &llama.Server{
			Port: cfg.EmbedPort, ModelPath: cfg.EmbedModel,
			Embedding: true,
		},
		Chat: &llama.Server{
			Port: cfg.ChatPort, ModelPath: cfg.ChatModel,
			DraftModel: cfg.DraftModel,
			// f16 KV + flash attention (ADR-014). q8_0 KV saved ~315 MB but
			// cost ~40% generation latency on the CPU-only audit box, where
			// dequantizing the cache on every attention step has no hardware
			// acceleration. Server config is dev-UX only per ADR-003 (the
			// audit profiles the raw GGUF), so the memory cost is free of
			// score impact. NB: the real latency bottleneck is prompt-prefill
			// size, not KV format — see ADR-014.
			ExtraArgs: []string{"-c", fmt.Sprint(cfg.ChatCtxSize), "-fa", "on"},
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

// Options tunes one Ask.
type Options struct {
	Pidgin bool // force Pidgin-first answers regardless of question language
}

// sentenceBreakRe splits a question into sub-questions on sentence boundaries
// and major conjunctions that join independent legal questions.
var sentenceBreakRe = regexp.MustCompile(`\s+(?:and|also|plus)\s+(?i)(?:what|how|is|are|do|does|can|does|am|I|my|which|wetin|abeg|who)\b`)

// decomposeQuery splits a compound question into independent sub-queries
// when it spans multiple legal domains. Returns the original question as a
// single element if the question isn't compound.
func decomposeQuery(question string) []string {
	// Strategy: find sentence boundaries (?, !, .) first, then split
	// compound clauses joined by "and"/"also" when followed by question words.
	parts := sentenceBreakRe.Split(question, -1)
	if len(parts) <= 1 {
		return []string{question}
	}

	// Filter out fragments that are too short to be meaningful queries.
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.TrimRight(p, "?.!,")
		words := strings.Fields(p)
		if len(words) >= 4 {
			out = append(out, p)
		}
	}

	if len(out) <= 1 {
		return []string{question}
	}
	return out
}

// mergeChunks deduplicates search results by chunk ID, keeping the best
// score for each chunk.
func mergeChunks(batches [][]retrieve.Chunk) []retrieve.Chunk {
	seen := map[int64]float64{}
	for _, batch := range batches {
		for _, c := range batch {
			if existing, ok := seen[c.ID]; !ok || c.Score > existing {
				seen[c.ID] = c.Score
			}
		}
	}
	out := make([]retrieve.Chunk, 0, len(seen))
	for _, batch := range batches {
		for _, c := range batch {
			if best, ok := seen[c.ID]; ok && best == c.Score {
				out = append(out, c)
				delete(seen, c.ID) // emit once
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// Ask runs the full retrieval → route → compute/generate → verify pipeline.
// Every stage calls the matching Emitter callback so the CLI can log to
// stderr and the web UI can push SSE events.
func (a *App) Ask(question string, opts Options, em Emitter) (*Report, error) {
	report := &Report{Question: question}

	t0 := time.Now()
	var err error

	// Query decomposition for compound cross-domain questions.
	// A single embedding can't represent "do I pay tax on rent AND
	// can I ask for 2 years' rent upfront?" — we split, embed, search
	// each sub-query, and merge with dedup.
	subs := decomposeQuery(question)
	var chunks []retrieve.Chunk
	if len(subs) == 1 {
		vec, e := a.Embed.Embed(llama.QueryPrefix + question)
		if e != nil {
			return nil, fmt.Errorf("embedding: %w", e)
		}
		chunks, err = a.Store.Search(vec, question, a.Config.TopN)
		if err != nil {
			return nil, fmt.Errorf("retrieval: %w", err)
		}
	} else {
		// Normalize each sub-query individually — the original split
		// preserves Pidgin markers for detection, this step adds
		// statutory English expansions.
		prefixes := make([]string, len(subs))
		for i, s := range subs {
			prefixes[i] = llama.QueryPrefix + s
		}
		vecs, e := a.Embed.EmbedBatch(prefixes)
		if e != nil {
			return nil, fmt.Errorf("embedding batch: %w", e)
		}
		var batches [][]retrieve.Chunk
		for i, vec := range vecs {
			batch, e := a.Store.Search(vec, subs[i], a.Config.TopN)
			if e != nil {
				return nil, fmt.Errorf("retrieval sub-query %d: %w", i, e)
			}
			batches = append(batches, batch)
		}
		chunks = mergeChunks(batches)
		// Cap at TopN × len(subs) to give cross-domain answers more
		// source material without unbounded growth.
		if maxN := a.Config.TopN * len(subs); len(chunks) > maxN {
			chunks = chunks[:maxN]
		}
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
	system := answer.SystemPrompt
	if opts.Pidgin {
		system += answer.PidginDirective
	}
	messages := []llama.ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: answer.BuildUserMessage(question, chunks, a.Config.Pack)},
	}
	if os.Getenv("OFIN_DEBUG_PROMPT") != "" {
		n := len(messages[0].Content) + len(messages[1].Content)
		fmt.Fprintf(os.Stderr, "[prompt] %d chars (~%d tokens) — pack full-n=%d full-chars=%d tail-chars=%d\n",
			n, n/4, a.Config.Pack.FullN, a.Config.Pack.FullChars, a.Config.Pack.TailChars)
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

	verifier := &verify.Verifier{Corpus: a.Store, Embed: a.Embed, Resolve: a.Store.ResolveAct, Cache: a.verifyCache}
	results, uncited := verifier.VerifyAnswer(question, full)

	// One constrained regeneration pass when any claim failed.
	if failed := failedResults(results); len(failed) > 0 {
		report.Regenerated = true
		if em.Regenerating != nil {
			em.Regenerating(len(failed))
		}
		correction := answer.BuildCorrectionMessage(failed)
		regen := append(messages,
			llama.ChatMessage{Role: "assistant", Content: full},
			llama.ChatMessage{Role: "user", Content: correction},
		)
		// The regeneration prompt rides on top of the full SOURCES block
		// and must fit the context window (observed: 6383 tokens vs 6144
		// ctx on a tax question). Under pressure, drop the failed draft —
		// the correction quotes every failed claim verbatim.
		if promptBudgetExceeded(regen, a.Config.ChatCtxSize-a.Config.MaxTokens) {
			regen = append(messages[:len(messages):len(messages)],
				llama.ChatMessage{Role: "user", Content: correction})
		}
		full, err = a.Chat.ChatStream(regen, a.Config.MaxTokens, a.Config.Temp, onToken)
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

// promptBudgetExceeded estimates whether a chat request will overflow the
// token budget. 3.5 chars/token is deliberately pessimistic for statutory
// English (~4) — a false positive only costs the draft echo, a false
// negative kills the request with a 400.
func promptBudgetExceeded(msgs []llama.ChatMessage, budget int) bool {
	chars := 0
	for _, m := range msgs {
		chars += len(m.Content) + 16 // per-message template overhead
	}
	return float64(chars)/3.5 > float64(budget)
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
	case "paye_conceptual":
		return renderPAYEConceptual(outcome)
	case "termination_notice":
		return renderNoticeCard(outcome)
	case "tenancy_notice":
		return renderTenancyCard(outcome)
	case "redundancy":
		return renderRedundancyCard(outcome)
	}
	return ""
}

func renderPAYEConceptual(outcome router.Outcome) string {
	var b strings.Builder
	b.WriteString(`<div class="computation paye">`)
	b.WriteString(`<div class="conceptual">`)
	b.WriteString(simpleMarkdown(outcome.Rendered))
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)
	return b.String()
}

// simpleMarkdown converts basic markdown (bold, paragraphs) to HTML.
func simpleMarkdown(s string) string {
	// Bold: **text** → <strong>text</strong>
	s = strings.ReplaceAll(s, "**", "\x00") // placeholder for opening
	parts := strings.Split(s, "\x00")
	var b strings.Builder
	for i, part := range parts {
		if i%2 == 0 {
			b.WriteString(part)
		} else {
			b.WriteString("<strong>" + part + "</strong>")
		}
	}
	// Paragraphs: double newline → </p><p>
	s = b.String()
	b.Reset()
	paras := strings.Split(s, "\n\n")
	b.WriteString("<p>")
	for i, p := range paras {
		if i > 0 {
			b.WriteString("</p><p>")
		}
		b.WriteString(strings.ReplaceAll(strings.TrimSpace(p), "\n", "<br>"))
	}
	b.WriteString("</p>")
	return b.String()
}

func renderTenancyCard(outcome router.Outcome) string {
	var res rules.TenancyNoticeResult
	_ = json.Unmarshal([]byte(outcome.JSON), &res)
	var b strings.Builder
	b.WriteString(`<div class="computation tenancy">`)
	fmt.Fprintf(&b, `<p class="outcome">Default notice: <strong>%s</strong> %s</p>`,
		res.Notice, res.Citation)
	b.WriteString(`<ul>`)
	for _, n := range res.Notes {
		fmt.Fprintf(&b, `<li>%s %s</li>`, n.Label, n.Citation)
	}
	b.WriteString(`</ul>`)
	fmt.Fprintf(&b, `<p class="note">Jurisdiction: Lagos State only — other states have their own tenancy laws.</p>`)
	fmt.Fprintf(&b, `<p class="basis">%s. Computed deterministically.</p>`, res.AsAt)
	b.WriteString(`</div>`)
	return b.String()
}

func renderRedundancyCard(outcome router.Outcome) string {
	var res rules.RedundancyResult
	_ = json.Unmarshal([]byte(outcome.JSON), &res)
	var b strings.Builder
	b.WriteString(`<div class="computation redundancy">`)
	b.WriteString(`<p class="outcome">Redundancy entitlements [Labour Act 2004, s.20]</p>`)
	b.WriteString(`<ul>`)
	for _, r := range res.Rights {
		fmt.Fprintf(&b, `<li>%s %s</li>`, r.Label, r.Citation)
	}
	if res.Notice != nil {
		fmt.Fprintf(&b, `<li>Plus <strong>%s</strong> notice of termination (or payment in lieu) %s</li>`,
			res.Notice.Notice, res.Notice.Citation)
	}
	b.WriteString(`</ul>`)
	b.WriteString(`<p class="note">The Labour Act sets a duty to negotiate redundancy pay — not a fixed amount [s.20(1)(c)].</p>`)
	fmt.Fprintf(&b, `<p class="basis">%s. Computed deterministically.</p>`, res.AsAt)
	b.WriteString(`</div>`)
	return b.String()
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
