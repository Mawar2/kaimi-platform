// Package scorer implements the Scorer agent — the Zone 1 bid/no-bid scoring
// component that sits between Hunter and the Opportunity Queue.
//
// The Scorer takes an opportunity from the queue, pre-computes deterministic
// eligibility signals (NAICS match, competency tag overlap, past performance,
// SDB set-aside), then passes those signals as structured context to Gemini
// 2.5 Pro via Vertex AI to synthesize a 0–100 score with a BID/NO_BID/REVIEW
// recommendation and human-readable reasoning.
//
// Architecture note: The Scorer is a Zone 1 component. It runs in batch mode
// after Hunter, before opportunities are presented to the human for selection.
// Agents never call each other directly — the Scorer reads from the store and
// writes scored fields back, leaving the queue for the human dashboard.
//
// Key types:
//
//   - CapabilityProfile: describes the company's scoring capabilities (NAICS,
//     competency tags, past performance, SDB status).
//   - Scorer: interface for the scoring service; mockable in unit tests.
//   - GeminiScorer: Vertex AI / Gemini 2.5 Pro implementation of Scorer.
//   - Signals: pre-computed deterministic signals passed to the LLM.
//   - ScoreResult: raw structured output from the LLM.
//   - ScoreAndSave: converts a 0–100 raw score to 0.0–1.0 and writes all
//     scored fields back to the store.
package scorer
