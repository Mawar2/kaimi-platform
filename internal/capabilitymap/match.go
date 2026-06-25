package capabilitymap

import "strings"

// Match is the result of matching a capability map against an opportunity's text: which
// of the company's competencies, matching-vocabulary keywords, and mission domains appear
// in the solicitation. It is the evidence behind a "why this fits your capabilities"
// rationale — capability-aware qualification beyond the NAICS/set-aside gate.
type Match struct {
	Competencies []string `json:"competencies,omitempty"`
	Keywords     []string `json:"keywords,omitempty"`
	Domains      []string `json:"domains,omitempty"`
	// Coverage is the total distinct matched terms — a simple, explainable strength
	// signal (not a weighted score; the scorer phase (E) consumes the map more deeply).
	Coverage int `json:"coverage"`
}

// Minimum term lengths to count as a match. The guard rejects 1–2 char tokens ("AI",
// "IT") that would be noise, but allows 3-char terms: in federal solicitations the most
// distinctive matches are agency/program acronyms (DHS, CDM, ZTA, GIS), and whole-word
// boundary matching keeps them precise.
const (
	minCompetencyLen = 3
	minKeywordLen    = 3
	minDomainLen     = 3
)

// Match reports which of the map's competencies, keywords, and domains appear (case-
// insensitive substring) in text — typically an opportunity's title + agency + NAICS
// description. It is deterministic and offline (no LLM): an explainable first pass at
// capability-aware qualification. A nil map yields an empty Match.
func (cm *CapabilityMap) Match(text string) Match {
	if cm == nil {
		return Match{}
	}
	lower := strings.ToLower(text)
	var out Match

	matchInto := func(dst *[]string, seen map[string]bool, term string, minLen int) {
		t := strings.TrimSpace(term)
		if len([]rune(t)) < minLen {
			return
		}
		key := strings.ToLower(t)
		if seen[key] {
			return
		}
		// Whole-word(/phrase) match, not bare substring, so a keyword like "cloud"
		// matches "cloud migration" but not "cloudy" — the "why this fits" evidence
		// must be trustworthy, not noisy.
		if containsWord(lower, key) {
			*dst = append(*dst, t)
			seen[key] = true
		}
	}

	sc := map[string]bool{}
	for _, c := range cm.CoreCompetencies {
		matchInto(&out.Competencies, sc, c.Name, minCompetencyLen)
	}
	sk := map[string]bool{}
	for _, k := range cm.Keywords {
		matchInto(&out.Keywords, sk, k, minKeywordLen)
	}
	sd := map[string]bool{}
	for _, d := range cm.Domains {
		matchInto(&out.Domains, sd, d, minDomainLen)
	}
	out.Coverage = len(out.Competencies) + len(out.Keywords) + len(out.Domains)
	return out
}

// containsWord reports whether needle appears in haystack on word/phrase boundaries —
// i.e. the character immediately before and after the match is not ASCII-alphanumeric
// (or the match sits at a string edge). Both arguments are expected lowercased. This
// prevents substring false positives ("cloud" in "cloudy", "data" in "database") so the
// capability-match evidence stays trustworthy.
func containsWord(haystack, needle string) bool {
	if needle == "" {
		return false
	}
	for start := 0; ; {
		i := strings.Index(haystack[start:], needle)
		if i < 0 {
			return false
		}
		i += start
		end := i + len(needle)
		beforeOK := i == 0 || !isAlnumByte(haystack[i-1])
		afterOK := end == len(haystack) || !isAlnumByte(haystack[end])
		if beforeOK && afterOK {
			return true
		}
		start = i + 1
	}
}

// isAlnumByte reports whether b is an ASCII letter or digit. Non-ASCII bytes count as a
// boundary (conservative), which is fine for English solicitation text.
func isAlnumByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}
