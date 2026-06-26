// Package naics provides the official 2022 U.S. NAICS taxonomy — the edition in force in
// 2026 and the one SAM.gov filters on (its `ncode` parameter) — plus a search over it.
// Onboarding uses it for a typeahead so a tenant picks a REAL code with its canonical title,
// instead of free-typing a code that might be wrong/misformatted and silently break the hunt.
//
// The dataset is the Census Bureau "6-digit_2022_Codes" file, embedded so there is no
// external dependency or network call at runtime (matches the no-external-assets rule).
package naics

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
)

//go:embed naics_2022.json
var rawData []byte

// Code is one NAICS industry: a six-digit Code and its official Title.
type Code struct {
	Code  string `json:"code"`
	Title string `json:"title"`
}

var codes []Code

func init() {
	if err := json.Unmarshal(rawData, &codes); err != nil {
		panic("naics: embedded 2022 dataset is corrupt: " + err.Error())
	}
}

// All returns the full 2022 NAICS list. The slice must not be mutated.
func All() []Code { return codes }

// Lookup returns the Code for an exact six-digit code and whether it exists. Used to
// validate a submitted code against the official taxonomy.
func Lookup(code string) (Code, bool) {
	code = strings.TrimSpace(code)
	for _, c := range codes {
		if c.Code == code {
			return c, true
		}
	}
	return Code{}, false
}

// defaultLimit caps results when the caller passes limit <= 0.
const defaultLimit = 20

// Search returns up to limit NAICS codes matching q (case-insensitive), ranked best-first:
// exact code, then code prefix, then a title word starting with q, then a title substring.
// A blank q returns nil. This powers the onboarding typeahead.
func Search(q string, limit int) []Code {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil
	}
	if limit <= 0 {
		limit = defaultLimit
	}

	type scored struct {
		c    Code
		rank int
	}
	var hits []scored
	for _, c := range codes {
		title := strings.ToLower(c.Title)
		switch {
		case c.Code == q:
			hits = append(hits, scored{c, 0})
		case strings.HasPrefix(c.Code, q):
			hits = append(hits, scored{c, 1})
		case wordPrefix(title, q):
			hits = append(hits, scored{c, 2})
		case strings.Contains(title, q):
			hits = append(hits, scored{c, 3})
		}
	}
	// Stable sort by rank, then code, so results are deterministic.
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].rank != hits[j].rank {
			return hits[i].rank < hits[j].rank
		}
		return hits[i].c.Code < hits[j].c.Code
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]Code, len(hits))
	for i, h := range hits {
		out[i] = h.c
	}
	return out
}

// wordPrefix reports whether any whitespace-delimited word in s starts with prefix.
func wordPrefix(s, prefix string) bool {
	for _, w := range strings.Fields(s) {
		if strings.HasPrefix(w, prefix) {
			return true
		}
	}
	return false
}
