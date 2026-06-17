// Package zone2view holds the pure, presentation-agnostic derivation of a
// Zone 2 proposal's display state from its persisted ProposalStatus: the
// pipeline position and state (View), the human-facing status phrase
// (StatusPhrase), the five stage names, and the criteria matcher
// (RequirementAddressed).
//
// It is the single source of truth shared by every surface that renders a
// proposal — the web dashboard (internal/dashboard, HTML templates) and the
// desktop app (internal/desktop, JSON view-models for the Wails webview) — so
// the two can never disagree about what stage a proposal is in or whether a
// must-have is addressed (issues #246 B2/B6, #249). It returns plain data only;
// each surface owns its own rendering.
package zone2view
