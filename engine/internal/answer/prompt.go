// Package answer builds the citation-enforcing prompt from retrieved chunks.
// The format follows the Week-1 bake-off grounded questions (eval/bakeoff/),
// which Llama 3.2 3B handled well at temp 0.2.
package answer

import (
	"fmt"
	"strings"

	"ofin/internal/retrieve"
)

const SystemPrompt = `You are Òfin, a Nigerian legal information assistant. Follow these rules strictly:
1. Answer ONLY from the provided statute text. Never use outside knowledge.
2. Every legal claim must end with a citation in the exact format [Act name, s.X] or [Act name, s.X(Y)], matching the SOURCE labels.
3. Answer whatever the sources DO establish, even partially. "What can I do?" means: explain the rights and entitlements the provided sections give the user in their situation. Only if NOTHING in the sources bears on the question, say: "The retrieved sections do not answer this question." Never guess beyond the sources.
4. If a source is state law (jurisdiction noted), say which state it applies to.
5. Users may ask in English or Nigerian Pidgin. Understand both; reply in the language the user used. A Pidgin question about being sacked ("dem sack me") is a question about termination of employment.
6. Be concise and practical. You provide legal information, not legal advice.`

// Per-source cap protects prefill latency. 3000 keeps a full Labour Act
// s.11 (incl. the payment-in-lieu subsection) intact; Week 5's prompt diet
// revisits this against target-hardware prefill numbers.
const maxChunkChars = 3000

// BuildUserMessage assembles the SOURCES + QUESTION message.
func BuildUserMessage(question string, chunks []retrieve.Chunk) string {
	var b strings.Builder
	b.WriteString("SOURCES:\n\n")
	for _, c := range chunks {
		title := ""
		if c.SectionTitle.Valid && c.SectionTitle.String != "" {
			title = " — " + c.SectionTitle.String
		}
		jur := ""
		if c.Jurisdiction != "federal" {
			jur = fmt.Sprintf(" (jurisdiction: %s)", c.Jurisdiction)
		}
		text := c.Text
		if len(text) > maxChunkChars {
			text = text[:maxChunkChars] + " …"
		}
		summary := ""
		if c.Summary != "" {
			summary = "Summary: " + c.Summary + "\n"
		}
		fmt.Fprintf(&b, "SOURCE %s%s%s (as at %s):\n%s%s\n\n", c.Citation(), title, jur, c.AsAt, summary, text)
	}
	fmt.Fprintf(&b, "QUESTION: %s", question)
	return b.String()
}
