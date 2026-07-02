// Package retrieve implements hybrid retrieval over ofin.db: sqlite-vec
// vector search + FTS5 keyword search, fused with reciprocal rank fusion.
package retrieve

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
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
	db *sql.DB
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

// Search runs both legs and fuses them with reciprocal rank fusion:
// score(chunk) = Σ_legs 1/(rrfK + rank). Returns the top n chunks.
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
	ranked := make([]scored, 0, len(scores))
	for id, sc := range scores {
		ranked = append(ranked, scored{id, sc})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	if len(ranked) > n {
		ranked = ranked[:n]
	}

	chunks := make([]Chunk, 0, len(ranked))
	for _, r := range ranked {
		c, err := s.chunkByID(r.id)
		if err != nil {
			return nil, err
		}
		c.Score = r.score
		chunks = append(chunks, *c)
	}
	return chunks, nil
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
