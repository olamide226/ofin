package retrieve

import (
	"database/sql"
	"slices"
	"testing"
)

// refIndexStore builds a Store over an in-memory corpus of four chunks that
// exercise every edge class the resolver must handle.
func refIndexStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	// Each pooled connection would get its own empty :memory: database.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE chunks (
		id INTEGER PRIMARY KEY, act_short TEXT, section_id TEXT,
		cross_refs TEXT, source TEXT, text TEXT)`); err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		id       int64
		act, sec string
		refs     string
		text     string
	}{
		// 1: the cited target section.
		{1, "NMW Act 2019", "s.3", `[]`, "3. The national minimum wage..."},
		// 2: cross-act ACT-LEVEL citation (no section) — NTA s.58 cites the
		// whole Minimum Wage Act, prose name — plus SAC's quirky schedule
		// ref format ("s.Fourth Schedule").
		{2, "Nigeria Tax Act 2025", "s.58", `["Minimum Wage Act", "Nigeria Tax Act, s.Fourth Schedule"]`,
			"58. The income tax payable..."},
		// 3: same-act sectioned self-reference.
		{3, "Nigeria Tax Act 2025", "s.57", `["section 58 of this Act"]`, "57. Effective tax rate..."},
		// 4: cross-act SECTIONED citation, prose act name; plus a ref to an
		// act outside the corpus that must NOT mint an edge.
		{4, "NMW Act 2019", "s.4", `["Nigeria Tax Act, 2025, s.58", "Pension Reform Act 2014, s.58"]`,
			"4. Exemptions..."},
		// 5: the statutory Fourth Schedule sits at document-order sch.7 —
		// resolvable only through its text head, never its id.
		{5, "Nigeria Tax Act 2025", "sch.7", `[]`, "Fourth Schedule\n Section 58(1)\n Tax rates..."},
	}
	sources := map[string]string{
		"NMW Act 2019":         "National Minimum Wage Act 2019 (as amended 2024)",
		"Nigeria Tax Act 2025": "Nigeria Tax Act, 2025 (Act No. 7)",
	}
	for _, r := range rows {
		if _, err := db.Exec(
			`INSERT INTO chunks (id, act_short, section_id, cross_refs, source, text) VALUES (?,?,?,?,?,?)`,
			r.id, r.act, r.sec, r.refs, sources[r.act], r.text); err != nil {
			t.Fatal(err)
		}
	}
	return &Store{db: db}
}

// The bands live in the Fourth Schedule, chunked as sch.7 (document order).
// Forward: s.58's "s.Fourth Schedule" ref must resolve to it. Reverse:
// seeding from sch.7 must find s.58 citing it.
func TestScheduleOrdinalResolution(t *testing.T) {
	s := refIndexStore(t)
	ids := s.resolveRefIDs("Nigeria Tax Act 2025", "Nigeria Tax Act, s.Fourth Schedule")
	if !slices.Contains(ids, int64(5)) {
		t.Errorf("forward schedule ref = %v, want it to contain 5 (sch.7)", ids)
	}
	rev := s.reverseRefIDs("Nigeria Tax Act 2025", "sch.7")
	if !slices.Contains(rev, int64(2)) {
		t.Errorf("reverse schedule lookup = %v, want it to contain 2 (s.58)", rev)
	}
}

func TestReverseRefIDsCrossAct(t *testing.T) {
	s := refIndexStore(t)

	// Seed NMW s.3: reachable only through NTA s.58's act-level prose
	// citation of the "Minimum Wage Act" — the cross-domain hop edge.
	got := s.reverseRefIDs("NMW Act 2019", "s.3")
	if !slices.Contains(got, int64(2)) {
		t.Errorf("reverseRefIDs(NMW s.3) = %v, want it to contain 2 (NTA s.58 act-level citation)", got)
	}

	// Seed NTA s.58: same-act edge (chunk 3) must rank before the
	// cross-act sectioned edge (chunk 4).
	got = s.reverseRefIDs("Nigeria Tax Act 2025", "s.58")
	i3, i4 := slices.Index(got, int64(3)), slices.Index(got, int64(4))
	if i3 == -1 || i4 == -1 {
		t.Fatalf("reverseRefIDs(NTA s.58) = %v, want both 3 (same-act) and 4 (cross-act)", got)
	}
	if i3 > i4 {
		t.Errorf("same-act edge ranked after cross-act edge: %v", got)
	}
	// The "Pension Reform Act 2014, s.58" ref must not create an edge under
	// any corpus act — s.58 lookups must never include a bogus id.
	if slices.Contains(s.reverseRefIDs("NMW Act 2019", "s.58"), int64(4)) {
		t.Errorf("out-of-corpus act ref minted a bogus same-act edge")
	}
}

func TestResolveRefIDsSkipsUnknownActs(t *testing.T) {
	s := refIndexStore(t)
	if ids := s.resolveRefIDs("NMW Act 2019", "Pension Reform Act 2014, s.58"); len(ids) != 0 {
		t.Errorf("resolveRefIDs resolved an out-of-corpus act to %v, want none", ids)
	}
	// Prose cross-act ref resolves to the right chunk.
	if ids := s.resolveRefIDs("NMW Act 2019", "Nigeria Tax Act, 2025, s.58"); !slices.Contains(ids, int64(2)) {
		t.Errorf("resolveRefIDs(NTA s.58 prose ref) = %v, want [2]", ids)
	}
	// Self-reference stays in the seed act.
	if ids := s.resolveRefIDs("Nigeria Tax Act 2025", "section 57 of this Act"); !slices.Contains(ids, int64(3)) {
		t.Errorf("resolveRefIDs(self-ref s.57) = %v, want [3]", ids)
	}
}
