package proposal

import (
	"context"

	"github.com/Mawar2/Kaimi/internal/agent"
	"github.com/Mawar2/Kaimi/internal/finalreview"
	"github.com/Mawar2/Kaimi/internal/ingest"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/outline"
	"github.com/Mawar2/Kaimi/internal/writer"
)

// The Zone 2 agent contracts. proposal.Service is the single orchestrator that
// threads an Opportunity through these stages (issue #174 retired the parallel
// manager.Manager); the interfaces live here, with the orchestrator that uses
// them, so the real agents (and their stubs/mocks) drop in unchanged.

// Ingestor fetches an opportunity's solicitation attachments, stores them, and
// returns the resulting SolicitationDocs together with a filename -> extracted
// text map for downstream grounding. The concrete *ingest.Agent satisfies this.
type Ingestor interface {
	Ingest(ctx context.Context, opp *opportunity.Opportunity) ([]opportunity.SolicitationDoc, map[string]string, *agent.Result, error)
}

// OutlineRunner produces an outline and a Result for an opportunity, grounded on
// the ingested solicitation document text. The concrete *outline.Agent satisfies
// this.
type OutlineRunner interface {
	Run(ctx context.Context, opp *opportunity.Opportunity, documents map[string]string) (*outline.Outline, *agent.Result, error)
}

// WriterRunner produces a draft and a Result from a writer.Input.
// The concrete *writer.Agent satisfies this.
type WriterRunner interface {
	Run(ctx context.Context, in writer.Input) (string, *agent.Result, error)
}

// Reviewer runs the final pre-submission review.
// The concrete *finalreview.Agent satisfies this.
type Reviewer interface {
	Review(ctx context.Context, in finalreview.Input) (*agent.Result, error)
}

// Compile-time checks that the real Zone 2 agents satisfy these interfaces.
var (
	_ Ingestor      = (*ingest.Agent)(nil)
	_ OutlineRunner = (*outline.Agent)(nil)
	_ WriterRunner  = (*writer.Agent)(nil)
	_ Reviewer      = (*finalreview.Agent)(nil)
)
