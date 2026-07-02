// Package retrieve implements hybrid retrieval over ofin.db: sqlite-vec
// vector search + FTS5 keyword search, fused with reciprocal rank fusion.
package retrieve

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

func init() {
	sqlite_vec.Auto()
}

const (
	legDepth = 20 // candidates fetched per leg before fusion
	rrfK     = 60 // standard RRF damping constant
)

type Chunk struct {
	ID           int64
	ActShort     string
	SectionID    string
	SectionTitle sql.NullString
	Part         sql.NullString
	Jurisdiction string
	Source       string
	AsAt         string
	Text         string
	Summary      string
	CrossRefs    []string
	Score        float64
}

// Citation returns the canonical citation label for prompt blocks and,
// later, verifier lookups — e.g. "[Labour Act 2004, s.11]".
func (c *Chunk) Citation() string {
	return fmt.Sprintf("[%s, %s]", c.ActShort, c.SectionID)
}

type Store struct {
	db       *sql.DB
	actNames map[string]string // normalized name -> act_short, lazily built
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) vectorLeg(embedding []float32) ([]int64, error) {
	blob, err := sqlite_vec.SerializeFloat32(embedding)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT rowid FROM vec_chunks WHERE embedding MATCH ? AND k = ? ORDER BY distance`,
		blob, legDepth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectIDs(rows)
}

var wordRe = regexp.MustCompile(`[a-zA-Z][a-zA-Z']+`)

// ftsQuery turns free text (possibly Pidgin, possibly with punctuation FTS5
// would reject) into a safe OR query of its distinct terms.
func ftsQuery(question string) string {
	seen := map[string]bool{}
	var terms []string
	for _, w := range wordRe.FindAllString(strings.ToLower(question), -1) {
		if len(w) < 3 || seen[w] {
			continue
		}
		seen[w] = true
		terms = append(terms, fmt.Sprintf("%q", w))
		if len(terms) >= 12 {
			break
		}
	}
	return strings.Join(terms, " OR ")
}

func (s *Store) keywordLeg(question string) ([]int64, error) {
	q := ftsQuery(question)
	if q == "" {
		return nil, nil
	}
	rows, err := s.db.Query(
		`SELECT rowid FROM chunks_fts WHERE chunks_fts MATCH ? ORDER BY bm25(chunks_fts) LIMIT ?`,
		q, legDepth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectIDs(rows)
}

func collectIDs(rows *sql.Rows) ([]int64, error) {
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Search runs both legs, fuses them with reciprocal rank fusion
// (score = Σ_legs 1/(rrfK + rank)), then expands one hop across the SAC
// cross-reference edges: sections cited by the top-3 fused chunks join the
// candidate pool at a discounted score. Statutes answer through their
// exemptions and companion provisions (s.3 minimum wage ↔ s.4 exemptions),
// and the cross_refs edges encode exactly that structure. Returns top n.
func (s *Store) Search(embedding []float32, question string, n int) ([]Chunk, error) {
	vecIDs, err := s.vectorLeg(embedding)
	if err != nil {
		return nil, fmt.Errorf("vector leg: %w", err)
	}
	ftsIDs, err := s.keywordLeg(question)
	if err != nil {
		return nil, fmt.Errorf("keyword leg: %w", err)
	}

	scores := map[int64]float64{}
	for _, leg := range [][]int64{vecIDs, ftsIDs} {
		for rank, id := range leg {
			scores[id] += 1.0 / float64(rrfK+rank+1)
		}
	}
	type scored struct {
		id    int64
		score float64
	}
	rank := func() []scored {
		ranked := make([]scored, 0, len(scores))
		for id, sc := range scores {
			ranked = append(ranked, scored{id, sc})
		}
		sort.Slice(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
		return ranked
	}

	// One-hop cross-reference expansion from the top seeds. Hop candidates
	// get a bounded QUOTA (at most hopSlots of the final n), never score
	// competition: an earlier score-discount design let hop noise displace
	// genuine leg hits (recall regressed on the golden set), while a quota
	// can only ever cost the two weakest fused ranks.
	const hopSeeds, hopSlots = 3, 2
	ranked := rank()
	fusedTop := map[int64]bool{}
	for i, r := range ranked {
		if i >= n {
			break
		}
		fusedTop[r.id] = true
	}
	var hops []int64
	addHop := func(id int64) {
		if !fusedTop[id] && len(hops) < hopSlots && !slices.Contains(hops, id) {
			hops = append(hops, id)
		}
	}
	for i, r := range ranked {
		if i >= hopSeeds {
			break
		}
		seed, err := s.chunkByID(r.id)
		if err != nil {
			continue
		}
		// Reverse edges first — statutes cite backwards (the exemption
		// section cites the rule, never the other way), so a seed's
		// exemptions/provisos are the chunks citing it.
		for _, id := range s.reverseRefIDs(seed.ActShort, seed.SectionID) {
			addHop(id)
		}
		for _, ref := range seed.CrossRefs {
			for _, id := range s.resolveRefIDs(seed.ActShort, ref) {
				addHop(id)
			}
		}
	}

	// Hops EXTEND the context rather than competing for it: n fused chunks
	// plus up to hopSlots companions. Quota-displacement cost a genuine
	// rank-5 hit on the golden set; the extra prompt tokens are the cheaper
	// price (revisited in the Week-5 prompt diet).
	keep := min(n, len(ranked))
	final := make([]scored, 0, keep+len(hops))
	final = append(final, ranked[:keep]...)
	for i, id := range hops {
		final = append(final, scored{id, ranked[keep-1].score * float64(hopSlots-i) * 0.01})
	}

	chunks := make([]Chunk, 0, len(final))
	for _, r := range final {
		c, err := s.chunkByID(r.id)
		if err != nil {
			return nil, err
		}
		c.Score = r.score
		chunks = append(chunks, *c)
	}
	return chunks, nil
}

var refSectionRe = regexp.MustCompile(`(?i)\bs(?:ections?)?\.?\s*(\d+[A-Za-z]?)`)

// resolveRefIDs maps a SAC cross_ref string ("Labour Act 2004, s.81" or
// "section 4 of this Act") to chunk row ids. Refs without an act name
// resolve within the seed chunk's own act.
func (s *Store) resolveRefIDs(seedAct, ref string) []int64 {
	m := refSectionRe.FindStringSubmatch(ref)
	if m == nil {
		return nil
	}
	sectionID := "s." + strings.ToLower(m[1])
	act := seedAct
	if resolved, ok := s.ResolveAct(refSectionRe.ReplaceAllString(ref, "")); ok {
		act = resolved
	}
	rows, err := s.db.Query(
		`SELECT id FROM chunks WHERE act_short = ? AND section_id = ?`, act, sectionID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	ids, _ := collectIDs(rows)
	return ids
}

func (s *Store) chunkByID(id int64) (*Chunk, error) {
	row := s.db.QueryRow(
		`SELECT id, act_short, section_id, section_title, part, jurisdiction,
		        source, as_at, text, summary, cross_refs
		 FROM chunks WHERE id = ?`, id)
	var c Chunk
	var crossRefs string
	if err := row.Scan(&c.ID, &c.ActShort, &c.SectionID, &c.SectionTitle, &c.Part,
		&c.Jurisdiction, &c.Source, &c.AsAt, &c.Text, &c.Summary, &crossRefs); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(crossRefs), &c.CrossRefs)
	return &c, nil
}

var actNormRe = regexp.MustCompile(`[^a-z0-9 ]`)

func normalizeActName(s string) string {
	s = strings.ToLower(s)
	s = actNormRe.ReplaceAllString(s, " ")
	fields := strings.Fields(s)
	out := fields[:0]
	for _, f := range fields {
		if f != "the" {
			out = append(out, f)
		}
	}
	return strings.Join(out, " ")
}

// ResolveAct maps a prose act name from model output to the canonical
// act_short used in the corpus. Matching is normalized containment in
// either direction against both the short name and the formal source name
// ("National Minimum Wage Act, 2019" matches source "National Minimum Wage
// Act 2019 (as amended 2024)" -> "NMW Act 2019").
func (s *Store) ResolveAct(raw string) (string, bool) {
	if s.actNames == nil {
		rows, err := s.db.Query(`SELECT DISTINCT act_short, source FROM chunks`)
		if err != nil {
			return "", false
		}
		defer rows.Close()
		s.actNames = map[string]string{}
		for rows.Next() {
			var short, source string
			if rows.Scan(&short, &source) == nil {
				s.actNames[normalizeActName(short)] = short
				s.actNames[normalizeActName(source)] = short
			}
		}
	}
	cand := normalizeActName(raw)
	if cand == "" {
		return "", false
	}
	if short, ok := s.actNames[cand]; ok {
		return short, true
	}
	for norm, short := range s.actNames {
		if strings.Contains(norm, cand) || strings.Contains(cand, norm) {
			return short, true
		}
	}
	return "", false
}

// reverseRefIDs finds same-act chunks whose cross_refs cite the given
// section. The trailing quote in the LIKE pattern anchors the element end
// inside the JSON array, so "s.3" cannot match "s.31". Same-act only for
// now: SAC refs spell act names in prose ("National Minimum Wage Act
// 2019") that don't LIKE-match act_short; cross-act reverse edges arrive
// with the tax/tenancy corpora via name resolution.
func (s *Store) reverseRefIDs(act, sectionID string) []int64 {
	rows, err := s.db.Query(
		`SELECT id FROM chunks WHERE act_short = ? AND cross_refs LIKE ?`,
		act, `%`+sectionID+`"%`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	ids, _ := collectIDs(rows)
	if len(ids) > 4 {
		ids = ids[:4] // heavily-cited sections (interpretation etc.) would flood
	}
	return ids
}

// SectionText returns the full text of a cited section — the verifier's
// lookup (Week 3). Exposed now because the prompt builder also uses it.
func (s *Store) SectionText(actShort, sectionID string) (string, error) {
	rows, err := s.db.Query(
		`SELECT text FROM chunks WHERE act_short = ? AND section_id = ? ORDER BY chunk_type`,
		actShort, sectionID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return "", err
		}
		parts = append(parts, t)
	}
	if len(parts) == 0 {
		return "", sql.ErrNoRows
	}
	return strings.Join(parts, "\n\n"), rows.Err()
}
