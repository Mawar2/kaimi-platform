// Package capabilitymap builds and stores a structured, per-tenant understanding of a
// customer's business — the "Capability Map" — from their onboarding profile and the
// context documents they provide (capability statements, CPARS, past proposals, and
// later a referenced Drive folder).
//
// The map is the SHARED CONTEXT artifact the rest of Kaimi references: capability-aware
// qualification and scoring match a solicitation's requirements against it, and the
// Zone-2 drafting agents ground their writing in it. It deliberately holds a richer,
// evidence-backed view than the flat scoring profile (keyword tags + prose sentences):
// named competencies with evidence, differentiators, mission domains, certifications,
// and an expanded matching vocabulary — so matching can be semantic rather than a naive
// substring overlap.
//
// Two builders implement the same interface: a DeterministicBuilder (offline/dev/tests,
// profile-only) and a GeminiBuilder (Vertex AI, profile + document text). The map is
// per-tenant and persisted via Store; nothing here crosses tenant boundaries.
package capabilitymap

import (
	"context"
	"strings"
	"time"

	"github.com/Mawar2/Kaimi/internal/profile"
)

// CapabilityMap is the structured business understanding shared across agents. All
// fields are additive/omitempty so the persisted JSON evolves without migrations.
type CapabilityMap struct {
	Company          string               `json:"company"`
	Summary          string               `json:"summary,omitempty"`           // 2–3 sentence business summary
	CoreCompetencies []Competency         `json:"core_competencies,omitempty"` // named, evidence-backed
	Differentiators  []string             `json:"differentiators,omitempty"`   // what sets them apart
	Domains          []string             `json:"domains,omitempty"`           // mission / customer domains served
	PastPerformance  []PastPerformanceRef `json:"past_performance,omitempty"`
	Certifications   []string             `json:"certifications,omitempty"` // set-asides, ISO/CMMC, clearances
	NAICS            []string             `json:"naics,omitempty"`          // codes the company genuinely serves
	Keywords         []string             `json:"keywords,omitempty"`       // expanded vocabulary for matching
	Sources          []string             `json:"sources,omitempty"`        // provenance (onboarding, doc names)
	Model            string               `json:"model"`                    // "deterministic" or the LLM model id
	GeneratedAt      time.Time            `json:"generated_at"`
}

// Competency is one capability area, ideally with evidence drawn from the company's
// documents or past performance so downstream matching and drafting can cite it.
type Competency struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Evidence    []string `json:"evidence,omitempty"`
}

// PastPerformanceRef is a relevant prior engagement, used both as scoring evidence and
// as grounding for proposal drafting.
type PastPerformanceRef struct {
	Client    string `json:"client,omitempty"`
	Scope     string `json:"scope,omitempty"`
	Value     string `json:"value,omitempty"`
	Relevance string `json:"relevance,omitempty"`
}

// ContextDoc is one extracted context document (name + plain text) fed into the builder.
// The text is the already-extracted content (the caller handles upload + extraction).
type ContextDoc struct {
	Name string
	Text string
}

// Builder turns a company profile plus context documents into a CapabilityMap.
// Implementations: DeterministicBuilder (offline) and GeminiBuilder (Vertex AI).
type Builder interface {
	Build(ctx context.Context, p *profile.CapabilityProfile, docs []ContextDoc) (*CapabilityMap, error)
}

// DeterministicBuilder produces a CapabilityMap from the onboarding profile alone, with
// no LLM call. It is the offline/dev/test builder and the safe fallback when no Vertex
// project is configured: the map is thinner (no document synthesis or evidence mining)
// but always available and deterministic. Context documents only contribute their names
// to Sources here — extracting meaning from them is the GeminiBuilder's job.
type DeterministicBuilder struct {
	// now is the clock, injectable for deterministic tests. Defaults to time.Now.
	now func() time.Time
}

// NewDeterministicBuilder returns an offline builder using the real clock.
func NewDeterministicBuilder() *DeterministicBuilder {
	return &DeterministicBuilder{now: time.Now}
}

// Build assembles a profile-only map. It never errors (a nil profile yields an empty
// map), so it is a dependable fallback.
func (b *DeterministicBuilder) Build(_ context.Context, p *profile.CapabilityProfile, docs []ContextDoc) (*CapabilityMap, error) {
	clock := b.now
	if clock == nil {
		clock = time.Now
	}
	m := &CapabilityMap{Model: "deterministic", GeneratedAt: clock().UTC()}
	if p == nil {
		return m, nil
	}

	m.Company = p.Company
	m.NAICS = naicsCodes(p)
	m.Certifications = setAsideCertifications(p.SetAside)
	m.CoreCompetencies = competencies(p)
	m.PastPerformance = pastPerformance(p)
	m.Keywords = keywords(p)
	m.Summary = summarize(p, m)

	m.Sources = append(m.Sources, "onboarding profile")
	for _, d := range docs {
		if name := strings.TrimSpace(d.Name); name != "" {
			m.Sources = append(m.Sources, name)
		}
	}
	return m, nil
}

// naicsCodes returns the profile's NAICS codes (trimmed, non-empty).
func naicsCodes(p *profile.CapabilityProfile) []string {
	out := make([]string, 0, len(p.NAICSCodes))
	for _, nc := range p.NAICSCodes {
		if c := strings.TrimSpace(nc.Code); c != "" {
			out = append(out, c)
		}
	}
	return out
}

// setAsideCertifications maps the boolean set-aside flags to human-readable labels.
func setAsideCertifications(sa profile.SetAsideStatus) []string {
	var out []string
	add := func(ok bool, label string) {
		if ok {
			out = append(out, label)
		}
	}
	add(sa.SmallBusiness, "Small Business")
	add(sa.SDB, "Small Disadvantaged Business (SDB)")
	add(sa.MinorityOwned, "Minority-Owned")
	add(sa.EightA, "8(a)")
	add(sa.SDVOSB, "SDVOSB")
	add(sa.WOSB, "WOSB")
	add(sa.HUBZone, "HUBZone")
	return out
}

// competencies builds named competencies from the profile's competency list and the
// scoring competency tags, de-duplicated case-insensitively. Evidence is empty here —
// only the GeminiBuilder mines evidence from documents.
func competencies(p *profile.CapabilityProfile) []Competency {
	seen := map[string]bool{}
	var out []Competency
	addName := func(name string) {
		n := strings.TrimSpace(name)
		if n == "" {
			return
		}
		k := strings.ToLower(n)
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, Competency{Name: n})
	}
	for _, c := range p.Competencies {
		addName(c)
	}
	for _, t := range p.Scoring.CompetencyTags {
		addName(t)
	}
	return out
}

// pastPerformance copies the profile's past-performance records into the map's shape.
func pastPerformance(p *profile.CapabilityProfile) []PastPerformanceRef {
	out := make([]PastPerformanceRef, 0, len(p.PastPerformance))
	for _, pp := range p.PastPerformance {
		if strings.TrimSpace(pp.Client) == "" && strings.TrimSpace(pp.Scope) == "" {
			continue
		}
		out = append(out, PastPerformanceRef{Client: pp.Client, Scope: pp.Scope, Value: pp.Value})
	}
	return out
}

// keywords builds the matching vocabulary from competency tags + competencies + NAICS
// descriptions, de-duplicated case-insensitively.
func keywords(p *profile.CapabilityProfile) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		v := strings.TrimSpace(s)
		if v == "" {
			return
		}
		k := strings.ToLower(v)
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, v)
	}
	for _, t := range p.Scoring.CompetencyTags {
		add(t)
	}
	for _, c := range p.Competencies {
		add(c)
	}
	for _, nc := range p.NAICSCodes {
		add(nc.Description)
	}
	return out
}

// summarize writes a plain one-liner business summary from the structured fields. The
// GeminiBuilder replaces this with a real synthesized summary; this keeps the offline
// map self-describing.
func summarize(p *profile.CapabilityProfile, m *CapabilityMap) string {
	name := strings.TrimSpace(p.Company)
	if name == "" {
		name = "This company"
	}
	var b strings.Builder
	b.WriteString(name)
	b.WriteString(" is a federal contractor")
	if len(m.NAICS) > 0 {
		b.WriteString(" serving NAICS ")
		b.WriteString(strings.Join(m.NAICS, ", "))
	}
	if len(m.CoreCompetencies) > 0 {
		names := make([]string, 0, len(m.CoreCompetencies))
		for _, c := range m.CoreCompetencies {
			names = append(names, c.Name)
		}
		b.WriteString(", with competencies in ")
		b.WriteString(strings.Join(names, ", "))
	}
	b.WriteString(".")
	return b.String()
}
