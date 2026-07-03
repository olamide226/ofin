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

	// Resolved cross-reference index, built lazily from every chunk's SAC
	// cross_refs. Keys carry canonical act_short, so lookups work across
	// acts even though the refs spell act names in prose.
	revBySection map[string][]revEdge // "act|s.N" -> chunks citing that section
	revByAct     map[string][]revEdge // act -> cross-act chunks citing the whole act
}

// revEdge is one citing chunk in the reverse-reference index.
type revEdge struct {
	id  int64
	act string // the citing chunk's act
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
	// can only ever cost the weakest fused ranks.
	//
	// Slots fill BREADTH-FIRST across seeds (each seed's best hop before any
	// seed's second): depth-first let the rank-1 seed consume every slot,
	// starving the seed that actually held the needed edge (eval L19: s.81
	// at rank 2 cites s.80, but both slots were spent on the rank-1 seed).
	const hopSeeds, hopSlots = 5, 3
	ranked := rank()
	fusedTop := map[int64]bool{}
	for i, r := range ranked {
		if i >= n {
			break
		}
		fusedTop[r.id] = true
	}
	var perSeed [][]int64
	for i, r := range ranked {
		if i >= hopSeeds {
			break
		}
		seed, err := s.chunkByID(r.id)
		if err != nil {
			continue
		}
		var mine []int64
		addMine := func(id int64) {
			if !fusedTop[id] && !slices.Contains(mine, id) {
				mine = append(mine, id)
			}
		}
		// Reverse edges first — statutes cite backwards (the exemption
		// section cites the rule, never the other way), so a seed's
		// exemptions/provisos are the chunks citing it.
		for _, id := range s.reverseRefIDs(seed.ActShort, seed.SectionID) {
			addMine(id)
		}
		for _, ref := range seed.CrossRefs {
			for _, id := range s.resolveRefIDs(seed.ActShort, ref) {
				addMine(id)
			}
		}
		perSeed = append(perSeed, mine)
	}
	var hops []int64
collect:
	for round := 0; ; round++ {
		progressed := false
		for _, mine := range perSeed {
			if round >= len(mine) {
				continue
			}
			progressed = true
			if id := mine[round]; !slices.Contains(hops, id) {
				hops = append(hops, id)
				if len(hops) == hopSlots {
					break collect
				}
			}
		}
		if !progressed {
			break
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

// refStopwords are words that appear in self-references ("section 4 of this
// Act") — a ref whose act-name part is nothing but these points back at the
// seed chunk's own act.
var refStopwords = map[string]bool{
	"act": true, "law": true, "this": true, "of": true, "under": true,
	"to": true, "pursuant": true, "section": true, "sections": true,
	"and": true, "or": true, "see": true, "cap": true, "cf": true,
	"chapter": true, "part": true, "schedule": true, "subsection": true,
}

var digitsRe = regexp.MustCompile(`^\d+$`)

// refTargetAct resolves which act a cross_ref points at, given the ref text
// with the section pattern stripped. Three outcomes: the seed act for
// self-references, a canonical act_short for prose names that resolve, and
// "" for refs naming an act outside the corpus — those edges must be
// SKIPPED, not defaulted to the seed act (a "Pension Reform Act 2014, s.4"
// ref must never mint a bogus edge to the seed act's own s.4).
func (s *Store) refTargetAct(seedAct, rest string) string {
	norm := normalizeActName(rest)
	substantive := false
	for _, w := range strings.Fields(norm) {
		if !refStopwords[w] && !digitsRe.MatchString(w) {
			substantive = true
			break
		}
	}
	if !substantive {
		return seedAct
	}
	if resolved, ok := s.ResolveAct(norm); ok {
		return resolved
	}
	return ""
}

// buildRefIndex resolves every chunk's cross_refs into the reverse index.
// One pass over the corpus (678 chunks) at first hop use; act names resolve
// through the same refTargetAct semantics as the forward direction.
func (s *Store) buildRefIndex() {
	s.revBySection = map[string][]revEdge{}
	s.revByAct = map[string][]revEdge{}
	rows, err := s.db.Query(`SELECT id, act_short, cross_refs FROM chunks`)
	if err != nil {
		return
	}
	// Collect before resolving: refTargetAct queries the DB (ResolveAct),
	// and nested queries inside an open result set are pool-dependent.
	type chunkRefs struct {
		id       int64
		act      string
		refsJSON string
	}
	var all []chunkRefs
	for rows.Next() {
		var c chunkRefs
		if rows.Scan(&c.id, &c.act, &c.refsJSON) == nil {
			all = append(all, c)
		}
	}
	rows.Close()
	for _, c := range all {
		id, act := c.id, c.act
		var refs []string
		_ = json.Unmarshal([]byte(c.refsJSON), &refs)
		for _, ref := range refs {
			m := refSectionRe.FindStringSubmatch(ref)
			rest := ref
			if m != nil {
				rest = refSectionRe.ReplaceAllString(ref, "")
			}
			target := s.refTargetAct(act, rest)
			if target == "" {
				continue // names an act outside the corpus
			}
			edge := revEdge{id: id, act: act}
			if m != nil {
				key := target + "|s." + strings.ToLower(m[1])
				s.revBySection[key] = append(s.revBySection[key], edge)
			} else if target != act {
				// Act-level citation of another act (e.g. NTA s.58 ->
				// "Minimum Wage Act"). Same-act bare refs carry no signal.
				s.revByAct[target] = append(s.revByAct[target], edge)
			}
		}
	}
}

// resolveRefIDs maps a SAC cross_ref string ("Labour Act 2004, s.81" or
// "section 4 of this Act") to chunk row ids. Prose act names resolve to
// canonical act_short; refs to acts outside the corpus resolve to nothing.
func (s *Store) resolveRefIDs(seedAct, ref string) []int64 {
	m := refSectionRe.FindStringSubmatch(ref)
	if m == nil {
		return nil
	}
	act := s.refTargetAct(seedAct, refSectionRe.ReplaceAllString(ref, ""))
	if act == "" {
		return nil
	}
	sectionID := "s." + strings.ToLower(m[1])
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

// reverseRefIDs finds chunks whose cross_refs cite the given section — in
// ANY act. Ranked: same-act sectioned edges first (a section's own
// exemptions and provisos), then cross-act sectioned edges, then act-level
// cross-act edges (a statute citing the whole act, e.g. NTA s.58 citing the
// "Minimum Wage Act" — the edge cross-domain questions travel). Capped:
// heavily-cited sections (interpretation etc.) would flood.
func (s *Store) reverseRefIDs(act, sectionID string) []int64 {
	if s.revBySection == nil {
		s.buildRefIndex()
	}
	var same, cross []int64
	for _, e := range s.revBySection[act+"|"+sectionID] {
		if e.act == act {
			same = append(same, e.id)
		} else {
			cross = append(cross, e.id)
		}
	}
	ids := append(same, cross...)
	for _, e := range s.revByAct[act] {
		ids = append(ids, e.id)
	}
	if len(ids) > 4 {
		ids = ids[:4]
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
