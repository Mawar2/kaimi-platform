// Package opportunity defines the core Opportunity schema that flows through
// the entire Kaimi pipeline.
//
// The Opportunity struct is the spine of the system. The Hunter creates it,
// the Scorer enriches it with bid/no-bid scoring, and the Zone 2 agents
// (Manager, Outline, Writer, Final Review) progressively build the proposal
// sections.
//
// IMPORTANT: This schema is designed for ALL phases, even though Phase 0 only
// populates the Hunter fields. Changing this schema later is the highest
// integration risk in the project, so we design it eagerly to be forward-compatible.
package opportunity

import (
	"time"
)

// Opportunity represents a federal contracting opportunity from discovery through
// proposal completion.
//
// Fields are grouped by the agent that populates them. Not all fields are populated
// at all stages - downstream agents progressively enrich the opportunity.
type Opportunity struct {
	// Core identification (populated by Hunter)
	ID               string    `json:"id"`                // SAM.gov notice ID
	Title            string    `json:"title"`             // Opportunity title
	SolicitationNum  string    `json:"solicitation_num"`  // Solicitation number
	Agency           string    `json:"agency"`            // Issuing agency
	Office           string    `json:"office"`            // Specific office within agency
	PostedDate       time.Time `json:"posted_date"`       // When opportunity was posted
	ResponseDeadline time.Time `json:"response_deadline"` // Proposal due date

	// Classification (populated by Hunter)
	NAICSCode          string `json:"naics_code"`           // Primary NAICS code
	NAICSDescription   string `json:"naics_description"`    // NAICS code description
	SetAsideCode       string `json:"set_aside_code"`       // Set-aside type (e.g., "SBA", "8A", "WOSB")
	PlaceOfPerformance string `json:"place_of_performance"` // Location of work

	// Opportunity details (populated by Hunter)
	Description  string `json:"description"`   // Full opportunity description
	Type         string `json:"type"`          // Opportunity type (e.g., "Solicitation", "Presolicitation")
	ContractType string `json:"contract_type"` // Contract type (e.g., "Firm Fixed Price", "T&M")

	// Links and attachments (populated by Hunter)
	URL         string   `json:"url"`         // Link to SAM.gov opportunity page
	Attachments []string `json:"attachments"` // URLs to attached documents (RFPs, etc.)

	// ResolvedDescription is the solicitation's full description TEXT. The SAM v2 search
	// API returns Description as a `noticedesc` URL, not prose; a resolver fetches that
	// URL → text and stores it here for the eligible set only (SAM-quota-bounded). Additive
	// and omitempty: older records simply omit it. Use EffectiveDescription to read.
	ResolvedDescription string `json:"resolved_description,omitempty"`

	// Scoring (populated by Scorer in Phase 1)
	Score          float64    `json:"score"`                    // Bid/no-bid score (0.0-1.0)
	ScoreReasoning string     `json:"score_reasoning"`          // LLM's reasoning for the score
	Recommendation string     `json:"recommendation,omitempty"` // Bid/no-bid recommendation: BID, NO_BID, or REVIEW
	Requirements   []string   `json:"requirements,omitempty"`   // Must-have requirements extracted from the solicitation
	ScoredAt       *time.Time `json:"scored_at,omitempty"`      // When scoring completed

	// Selection and status (populated by selection event / Manager)
	Selected       bool       `json:"selected"`              // Whether a human selected this for proposal
	SelectedAt     *time.Time `json:"selected_at,omitempty"` // When selected
	ProposalStatus string     `json:"proposal_status"`       // Current status in Zone 2 (e.g., "outline", "draft", "review")
	// ProposalStatusReason is a short, single-line, human-readable explanation set
	// when ProposalStatus ends in ":failed" (e.g. "writer:failed"), so the dashboard
	// and API can show WHY a proposal stalled. Empty on success/in-progress. Additive
	// and omitempty: legacy records without it load fine.
	ProposalStatusReason string `json:"proposal_status_reason,omitempty"`

	// Award tracking + contract value, surfaced by the Submitted archive screen.
	// All additive and omitempty, so the JSON store needs no migration and older
	// records simply omit them (the UI degrades to "—" / UpdatedAt / pending).
	EstimatedValue float64    `json:"estimated_value,omitempty"` // contract value in US dollars (0 = unknown)
	SubmittedAt    *time.Time `json:"submitted_at,omitempty"`    // when the human submitted to SAM.gov
	AwardOutcome   string     `json:"award_outcome,omitempty"`   // "" (pending) | "won" | "lost"

	// Solicitation documents (populated by the Manager's ingest stage in Zone 2)
	//
	// Attachments (above) holds the original SAM.gov attachment URLs the Hunter
	// found. Documents is the post-ingestion, enriched set: each entry records where
	// the raw file and the extracted text live in GCS, so users can re-download the
	// originals and downstream agents (Outline, Writer, Final Review) can ground on
	// the real document text rather than the SAM.gov summary alone. Empty until the
	// ingest stage runs.
	Documents []SolicitationDoc `json:"documents,omitempty"`

	// Proposal sections (populated by Zone 2 agents in Phase 3)
	// TODO(phase-3): Add outline, technical approach, past performance, etc.
	// Outline         *ProposalOutline `json:"outline,omitempty"`
	// TechnicalDraft  string           `json:"technical_draft,omitempty"`
	// ReviewedDraft   string           `json:"reviewed_draft,omitempty"`

	// Metadata
	TenantID  string    `json:"tenant_id,omitempty"` // owning deployment/org; empty on legacy records
	CreatedAt time.Time `json:"created_at"`          // When opportunity was first saved
	UpdatedAt time.Time `json:"updated_at"`          // Last update timestamp
}

// EffectiveDescription returns the resolved solicitation text when it has been fetched,
// otherwise the raw Description. The Scorer (and capability-match) read this so they work
// against real prose rather than the SAM `noticedesc` URL that the search API returns in
// Description. Behavior is unchanged for records that have no resolved text yet.
func (o *Opportunity) EffectiveDescription() string {
	if o.ResolvedDescription != "" {
		return o.ResolvedDescription
	}
	return o.Description
}

// SolicitationDoc records one solicitation attachment after it has been ingested:
// fetched from its source URL, stored in GCS, and had its text extracted. The
// Manager's ingest stage (Ticket C) populates these and attaches them to the
// Opportunity; the Store persists them.
//
// The struct holds GCS object references, not the bytes or extracted text
// themselves — keeping the Opportunity small enough to persist in a document store
// (e.g. Firestore's 1 MB per-document limit). Consumers read the raw file from
// RawObject and the extracted text from TextObject.
type SolicitationDoc struct {
	Filename    string    `json:"filename"`     // Original attachment filename (e.g., "RFP_Section_L.pdf")
	SourceURL   string    `json:"source_url"`   // SAM.gov URL the document was fetched from
	ContentType string    `json:"content_type"` // MIME type as served (e.g., "application/pdf")
	RawObject   string    `json:"raw_object"`   // gs:// path to the raw downloaded file (user re-download)
	TextObject  string    `json:"text_object"`  // gs:// path to the extracted plain text (agent grounding)
	SHA256      string    `json:"sha256"`       // Hex SHA-256 of the raw bytes (dedup + change detection)
	Bytes       int64     `json:"bytes"`        // Size of the raw file in bytes
	IngestedAt  time.Time `json:"ingested_at"`  // When the document was fetched and stored
}

// TODO(phase-3): Define ProposalOutline struct when Outline agent is built.
// type ProposalOutline struct {
//     ExecutiveSummary string
//     TechnicalApproach []Section
//     PastPerformance []Section
//     ManagementPlan string
// }
