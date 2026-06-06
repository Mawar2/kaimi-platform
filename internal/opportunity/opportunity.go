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

	// Proposal sections (populated by Zone 2 agents in Phase 3)
	// TODO(phase-3): Add outline, technical approach, past performance, etc.
	// Outline         *ProposalOutline `json:"outline,omitempty"`
	// TechnicalDraft  string           `json:"technical_draft,omitempty"`
	// ReviewedDraft   string           `json:"reviewed_draft,omitempty"`

	// Metadata
	CreatedAt time.Time `json:"created_at"` // When opportunity was first saved
	UpdatedAt time.Time `json:"updated_at"` // Last update timestamp
}

// TODO(phase-3): Define ProposalOutline struct when Outline agent is built.
// type ProposalOutline struct {
//     ExecutiveSummary string
//     TechnicalApproach []Section
//     PastPerformance []Section
//     ManagementPlan string
// }
