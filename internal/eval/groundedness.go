package eval

import (
	"context"
	"fmt"
	"strings"
	"unicode"
)

// gapMarker mirrors writer.gapMarker: the placeholder the Writer is instructed to
// emit when a fact is missing instead of fabricating it. A sentence carrying this
// marker is honest behavior, so groundedness treats it as grounded rather than as
// an unsupported claim. It is duplicated (not imported) to keep eval decoupled from
// the writer package's internals.
const gapMarker = "[GAP:"

// SectionDrafter is the consumer-side interface this harness needs from the Writer's
// generation path. It is identical in shape to writer.Generator, so the real
// *writer.GeminiGenerator satisfies it with no change to the writer package; the fast
// test layer injects a mock. We evaluate at the section-drafter level (rather than the
// whole writer.Agent.Run) because that is where we control the exact facts a section
// is allowed to use, which is precisely what groundedness measures.
type SectionDrafter interface {
	GenerateSection(ctx context.Context, systemInstruction, prompt string) (string, error)
}

// WriterCase is one groundedness evaluation case.
type WriterCase struct {
	// Name identifies the case in the report. Required.
	Name string
	// SectionPrompt is the user prompt handed to the drafter. Required.
	SectionPrompt string
	// SystemInstruction is the anti-fabrication system instruction. Optional; when
	// empty the harness supplies a default grounding instruction.
	SystemInstruction string
	// Facts are the only facts the section is allowed to assert (profile competencies,
	// past performance, opportunity details). Groundedness is measured against these.
	Facts []string
	// MustNotFabricate lists specific terms that, if asserted in the draft, indicate
	// fabrication — e.g. a contract number or certification the company does not hold.
	MustNotFabricate []string
}

// WriterCaseResult is the per-case groundedness outcome.
type WriterCaseResult struct {
	Name string `json:"name"`
	// Groundedness is the fraction of evaluated sentences supported by the facts,
	// in [0,1]. A section with no evaluable sentences scores 1.0 (nothing unsupported).
	Groundedness float64 `json:"groundedness"`
	// UngroundedClaims are the sentences whose significant tokens were not all found
	// in the supplied facts.
	UngroundedClaims []string `json:"ungrounded_claims,omitempty"`
	// FabricationDetected is true when any MustNotFabricate term appears in the draft.
	FabricationDetected bool `json:"fabrication_detected"`
	// FabricatedTerms lists which MustNotFabricate terms were found.
	FabricatedTerms []string `json:"fabricated_terms,omitempty"`
}

// WriterReport is the structured groundedness report for the Writer.
type WriterReport struct {
	Total int `json:"total"`
	// MeanGroundedness is the mean per-case Groundedness across the dataset.
	MeanGroundedness float64 `json:"mean_groundedness"`
	// FabricationCount is the number of cases where a must-not-fabricate term appeared.
	FabricationCount int                `json:"fabrication_count"`
	Cases            []WriterCaseResult `json:"cases"`
}

// defaultGroundingInstruction is used when a case supplies no SystemInstruction. It
// states the same anti-fabrication rule the Writer enforces in production.
const defaultGroundingInstruction = "Use ONLY the facts provided. Do not invent past " +
	"performance, contract numbers, client names, dollar amounts, certifications, or dates. " +
	"If a fact is missing, insert a placeholder of the form [GAP: what is missing] instead of fabricating it."

// EvaluateWriter drafts each case via the SectionDrafter and scores groundedness.
//
// It returns an error if the dataset is empty or the drafter fails on any case.
func EvaluateWriter(ctx context.Context, d SectionDrafter, cases []WriterCase) (*WriterReport, error) {
	if len(cases) == 0 {
		return nil, fmt.Errorf("eval: writer dataset is empty")
	}

	rep := &WriterReport{Total: len(cases), Cases: make([]WriterCaseResult, 0, len(cases))}
	var sum float64

	for _, c := range cases {
		sys := c.SystemInstruction
		if sys == "" {
			sys = defaultGroundingInstruction
		}

		// The system instruction says "use ONLY the facts provided", so the
		// facts must travel in the prompt itself — without them the model is
		// asked to ground on nothing and the score is meaningless (issue #254).
		prompt := c.SectionPrompt
		if len(c.Facts) > 0 {
			prompt += "\n\nFacts (the ONLY facts you may use):\n- " + strings.Join(c.Facts, "\n- ")
		}

		draft, err := d.GenerateSection(ctx, sys, prompt)
		if err != nil {
			return nil, fmt.Errorf("eval: drafter failed on case %q: %w", c.Name, err)
		}

		cr := scoreGroundedness(&c, draft)
		rep.Cases = append(rep.Cases, cr)
		sum += cr.Groundedness
		if cr.FabricationDetected {
			rep.FabricationCount++
		}
	}

	rep.MeanGroundedness = sum / float64(rep.Total)
	return rep, nil
}

// scoreGroundedness applies the v1 groundedness heuristic to a single draft.
//
// Heuristic (intentionally simple, explainable, and conservative for v1):
//
//  1. Split the draft into sentences.
//  2. A sentence is "grounded" if every one of its significant tokens (lowercased
//     alphanumeric words longer than three characters, minus a small stop-word set)
//     appears somewhere in the concatenated, lowercased facts. Honest gap markers
//     ([GAP: ...]) and sentences with no significant tokens count as grounded.
//  3. Groundedness = grounded sentences / evaluated sentences.
//  4. Separately, if any MustNotFabricate term appears in the draft, fabrication is
//     flagged regardless of the groundedness fraction.
//
// Known limits (documented so the numbers are not over-trusted): this is lexical, not
// semantic — it cannot tell that "the agency" refers to a named department, it will
// miss paraphrase and synonymy, and it can be fooled by facts that happen to share
// tokens. It is a regression tripwire to catch obvious fabrication and drift, not a
// substitute for Malik reading the draft. Treat scores as relative, not absolute truth.
func scoreGroundedness(c *WriterCase, draft string) WriterCaseResult {
	cr := WriterCaseResult{Name: c.Name, Groundedness: 1.0}

	// Fabrication check: literal, case-insensitive presence of forbidden terms.
	lowerDraft := strings.ToLower(draft)
	for _, term := range c.MustNotFabricate {
		if term == "" {
			continue
		}
		if strings.Contains(lowerDraft, strings.ToLower(term)) {
			cr.FabricationDetected = true
			cr.FabricatedTerms = append(cr.FabricatedTerms, term)
		}
	}

	factTokens := tokenSet(strings.Join(c.Facts, " "))

	var evaluated, grounded int
	for _, sentence := range splitSentences(draft) {
		// A gap marker is the correct, honest response to a missing fact.
		if strings.Contains(sentence, gapMarker) {
			evaluated++
			grounded++
			continue
		}

		toks := significantTokens(sentence)
		if len(toks) == 0 {
			// No claim to verify (e.g. boilerplate); do not penalize.
			evaluated++
			grounded++
			continue
		}

		evaluated++
		if allTokensSupported(toks, factTokens) {
			grounded++
		} else {
			cr.UngroundedClaims = append(cr.UngroundedClaims, strings.TrimSpace(sentence))
		}
	}

	if evaluated > 0 {
		cr.Groundedness = float64(grounded) / float64(evaluated)
	}
	return cr
}

// allTokensSupported reports whether every token is present in the fact token set.
func allTokensSupported(tokens []string, facts map[string]struct{}) bool {
	for _, t := range tokens {
		if _, ok := facts[t]; !ok {
			return false
		}
	}
	return true
}

// stopWords are common words ignored when deciding if a sentence is grounded. They
// carry no factual claim, so requiring them to appear in the facts would wrongly
// flag ordinary prose. Kept small and obvious on purpose.
var stopWords = map[string]struct{}{
	"the": {}, "and": {}, "for": {}, "with": {}, "that": {}, "this": {},
	"are": {}, "was": {}, "were": {}, "have": {}, "has": {}, "will": {},
	"our": {}, "their": {}, "from": {}, "into": {}, "they": {}, "them": {},
	"provide": {}, "provides": {}, "delivers": {}, "deliver": {}, "core": {},
	"performed": {}, "perform": {}, "include": {}, "includes": {}, "including": {},
	"only": {}, "won": {}, "award": {}, "match": {}, "strong": {},
}

// significantTokens returns the lowercased alphanumeric tokens of a sentence longer
// than three characters that are not stop words — the tokens whose support in the
// facts we actually check.
func significantTokens(sentence string) []string {
	var out []string
	for _, raw := range tokenize(sentence) {
		if len(raw) <= 3 {
			continue
		}
		if _, stop := stopWords[raw]; stop {
			continue
		}
		out = append(out, raw)
	}
	return out
}

// tokenSet returns the set of all lowercased alphanumeric tokens in text, including
// short tokens, so multi-word facts contribute every word as support.
func tokenSet(text string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, t := range tokenize(text) {
		set[t] = struct{}{}
	}
	return set
}

// tokenize lowercases text and splits it on any non-alphanumeric rune, dropping
// empties. It is the single shared tokenizer for both facts and draft sentences so
// the two sides are compared on identical terms.
func tokenize(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return fields
}

// splitSentences breaks a draft into sentences on '.', '!', and '?'. This is a coarse
// split (it does not handle abbreviations like "U.S."), which is acceptable for the v1
// heuristic — over-splitting only makes the groundedness check stricter, not looser.
func splitSentences(draft string) []string {
	var sentences []string
	var b strings.Builder
	for _, r := range draft {
		b.WriteRune(r)
		if r == '.' || r == '!' || r == '?' {
			if s := strings.TrimSpace(b.String()); s != "" {
				sentences = append(sentences, s)
			}
			b.Reset()
		}
	}
	if s := strings.TrimSpace(b.String()); s != "" {
		sentences = append(sentences, s)
	}
	return sentences
}
