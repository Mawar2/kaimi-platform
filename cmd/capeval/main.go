// Command capeval runs an offline A/B evaluation of the capability-map scoring upgrade:
// for a small sample of existing opportunities it scores arm A (today's behavior — the
// raw Description, which is a noticedesc URL, and profile-only signals) against arm B
// (the resolved solicitation TEXT plus capability-map signals), using the real Vertex
// GeminiScorer, and writes a markdown comparison.
//
// It is a one-off evaluation tool, NOT part of the live pipeline. It never writes back to
// any store and resolves descriptions only for the bounded sample it is given (SAM quota).
//
// Usage:
//
//	SAM_API_KEY=... go run ./cmd/capeval \
//	  -opps ./sample-opps -map ./capability_map.json -profile ./profile.json \
//	  -out ./docs/goals/capability-scoring-ab.md -limit 8
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Mawar2/Kaimi/internal/capabilitymap"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/profile"
	"github.com/Mawar2/Kaimi/internal/samgov"
	"github.com/Mawar2/Kaimi/internal/scorer"
)

func main() {
	oppsDir := flag.String("opps", "", "directory of opportunity JSON files")
	mapPath := flag.String("map", "", "capability_map.json path")
	profPath := flag.String("profile", "", "profile.json path")
	outPath := flag.String("out", "capability-scoring-ab.md", "markdown report output path")
	project := flag.String("project", "kaimi-seeker", "GCP project for Vertex")
	region := flag.String("region", "us-east4", "Vertex region")
	model := flag.String("model", "gemini-2.5-pro", "Gemini model")
	limit := flag.Int("limit", 8, "max opportunities to evaluate (SAM/Vertex cost bound)")
	flag.Parse()

	if err := run(*oppsDir, *mapPath, *profPath, *outPath, *project, *region, *model, *limit); err != nil {
		fmt.Fprintln(os.Stderr, "capeval:", err)
		os.Exit(1)
	}
}

// armResult is one scorer arm's output for an opportunity.
type armResult struct {
	score   int
	rec     string
	matched []string
	err     error
}

func run(oppsDir, mapPath, profPath, outPath, project, region, model string, limit int) error {
	ctx := context.Background()

	opps, err := loadOpps(oppsDir, limit)
	if err != nil {
		return err
	}
	if len(opps) == 0 {
		return fmt.Errorf("no opportunities found in %s", oppsDir)
	}

	capMap, err := loadMap(mapPath)
	if err != nil {
		return err
	}
	prof, err := loadProfile(profPath)
	if err != nil {
		return err
	}
	scoreProfile := scorer.FromProfile(prof)

	samKey := os.Getenv("SAM_API_KEY")
	var resolver *samgov.DescriptionResolver
	if samKey != "" {
		resolver, err = samgov.NewDescriptionResolver(samKey)
		if err != nil {
			return err
		}
	}

	gs, err := scorer.NewGeminiScorer(ctx, project, region, model)
	if err != nil {
		return fmt.Errorf("create Gemini scorer: %w", err)
	}

	var rows []reportRow
	for _, opp := range opps {
		fmt.Fprintf(os.Stderr, "scoring %s …\n", opp.ID)

		// Arm A: today's behavior — raw Description (a noticedesc URL), no map.
		a := scoreArm(ctx, gs.WithCapabilityMap(nil), opp, &scoreProfile)

		// Resolve the description to real text for arm B (bounded; only this sample).
		resolved, resolveNote := resolveDescription(ctx, resolver, opp)
		oppB := *opp
		oppB.ResolvedDescription = resolved

		// Arm B: resolved text + capability-map signals.
		b := scoreArm(ctx, gs.WithCapabilityMap(capMap), &oppB, &scoreProfile)
		// Surface the same capability matches the scorer saw (Score doesn't return signals).
		mt := capMap.Match(oppB.Title + " " + oppB.EffectiveDescription())
		b.matched = append(append(append([]string{}, mt.Competencies...), mt.Domains...), mt.Keywords...)

		rows = append(rows, reportRow{opp: opp, a: a, b: b, resolved: resolved != "", resolveNote: resolveNote})
	}

	report := buildReport(rows, capMap, resolver != nil, model)
	if err := os.WriteFile(outPath, []byte(report), 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%d opportunities)\n", outPath, len(rows))
	return nil
}

func scoreArm(ctx context.Context, gs *scorer.GeminiScorer, opp *opportunity.Opportunity, sp *scorer.CapabilityProfile) armResult {
	res, err := gs.Score(ctx, opp, sp)
	if err != nil {
		return armResult{err: err}
	}
	return armResult{score: res.RawScore, rec: string(res.Recommendation)}
}

func resolveDescription(ctx context.Context, r *samgov.DescriptionResolver, opp *opportunity.Opportunity) (text, note string) {
	if r == nil {
		return "", "no SAM key — arm B used the raw description"
	}
	if !strings.HasPrefix(opp.Description, "http") {
		return opp.Description, "description was already text"
	}
	text, err := r.Resolve(ctx, opp.Description)
	if err != nil {
		return "", "resolve failed: " + err.Error()
	}
	return text, ""
}

type reportRow struct {
	opp         *opportunity.Opportunity
	a, b        armResult
	resolved    bool
	resolveNote string
}

func buildReport(rows []reportRow, capMap *capabilitymap.CapabilityMap, samWired bool, model string) string {
	var sb strings.Builder
	company := "the tenant"
	if capMap != nil && capMap.Company != "" {
		company = capMap.Company
	}
	sb.WriteString("# Capability-map scoring A/B evaluation\n\n")
	fmt.Fprintf(&sb, "**Company:** %s · **Scorer:** %s (Vertex) · **Opportunities:** %d\n\n", company, model, len(rows))
	sb.WriteString("- **Arm A (current):** scores the raw `Description` (a SAM `noticedesc` URL) with profile-only signals.\n")
	sb.WriteString("- **Arm B (capability-map):** scores the RESOLVED solicitation text with capability-map coverage signals.\n\n")
	if !samWired {
		sb.WriteString("> ⚠️ No SAM key was provided, so descriptions could NOT be resolved — arm B differs from A only by the capability-map signals, not by real text. Re-run with `SAM_API_KEY` for the full comparison.\n\n")
	}

	// Aggregate stats.
	var deltaSum, recChanges, resolved int
	for i := range rows {
		r := &rows[i]
		if r.a.err == nil && r.b.err == nil {
			deltaSum += r.b.score - r.a.score
			if r.a.rec != r.b.rec {
				recChanges++
			}
		}
		if r.resolved {
			resolved++
		}
	}
	fmt.Fprintf(&sb, "**Headline:** %d/%d descriptions resolved to text; net score delta (B−A) = %+d total; recommendation changed on %d opportunit%s.\n\n",
		resolved, len(rows), deltaSum, recChanges, plural(recChanges))

	sb.WriteString("| Opportunity | A score/rec | B score/rec | Δ | B matched capabilities |\n")
	sb.WriteString("|---|---|---|---|---|\n")
	for i := range rows {
		r := &rows[i]
		title := r.opp.Title
		if len(title) > 48 {
			title = title[:47] + "…"
		}
		sb.WriteString("| " + mdEscape(title) + " | " + arm(r.a) + " | " + arm(r.b) + " | " + delta(r.a, r.b) + " | " + mdEscape(strings.Join(r.b.matched, ", ")) + " |\n")
	}
	sb.WriteString("\n## Per-opportunity notes\n\n")
	for i := range rows {
		r := &rows[i]
		fmt.Fprintf(&sb, "- **%s** (`%s`): ", mdEscape(r.opp.Title), shortID(r.opp.ID))
		switch {
		case r.a.err != nil || r.b.err != nil:
			fmt.Fprintf(&sb, "scoring error (A: %v, B: %v).", r.a.err, r.b.err)
		default:
			fmt.Fprintf(&sb, "A=%d/%s → B=%d/%s.", r.a.score, r.a.rec, r.b.score, r.b.rec)
		}
		if r.resolveNote != "" {
			sb.WriteString(" _" + r.resolveNote + "_")
		}
		sb.WriteString("\n")
	}
	fmt.Fprintf(&sb, "\n_Generated %s. One-off eval — not wired into the live pipeline. Cutover is Malik's decision._\n", time.Now().UTC().Format("2006-01-02 15:04 MST"))
	return sb.String()
}

func arm(r armResult) string {
	if r.err != nil {
		return "error"
	}
	return fmt.Sprintf("%d / %s", r.score, r.rec)
}

func delta(a, b armResult) string {
	if a.err != nil || b.err != nil {
		return "—"
	}
	return fmt.Sprintf("%+d", b.score-a.score)
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func mdEscape(s string) string { return strings.ReplaceAll(s, "|", "\\|") }

func loadOpps(dir string, limit int) ([]*opportunity.Opportunity, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read opps dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var opps []*opportunity.Opportunity
	for _, name := range names {
		if len(opps) >= limit {
			break
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		var opp opportunity.Opportunity
		if err := json.Unmarshal(data, &opp); err != nil {
			return nil, fmt.Errorf("decode %s: %w", name, err)
		}
		opps = append(opps, &opp)
	}
	return opps, nil
}

func loadMap(path string) (*capabilitymap.CapabilityMap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read map: %w", err)
	}
	var m capabilitymap.CapabilityMap
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("decode map: %w", err)
	}
	return &m, nil
}

func loadProfile(path string) (*profile.CapabilityProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}
	var p profile.CapabilityProfile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("decode profile: %w", err)
	}
	return &p, nil
}
