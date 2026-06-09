// Package finalreview implements the Final Review agent — the last automated
// checkpoint before a human submits a proposal.
//
// The Final Review agent sits at the end of the Zone 2 pipeline:
//
//	Manager → Outline → Technical Writer → [HUMAN GATE] → Final Review
//
// It receives the human-approved draft and its Opportunity, runs a set of
// automated checks, and returns an AgentResult indicating whether the proposal
// is ready for submission. Five checks are performed:
//
//   - deadline: expired deadline → StatusFailed; all other issues → StatusNeedsHuman
//   - must_have: each Opportunity.Requirements keyword must appear in the draft
//   - required_section: every Required=true Outline section must appear in the draft
//   - required_form: each FormattingRules.RequiredForms entry must be acknowledged
//   - page_limit: draft word count must not exceed the stated page limit (250 words/page)
//
// When the Input.Outline field is nil, only the deadline and must_have checks
// run — existing callers without an Outline are not broken.
//
// IMPORTANT: This agent NEVER submits anything. StatusReadyToSubmit in the
// returned AgentResult is a signal for a human to act on. No submission API
// is called by this agent.
package finalreview
