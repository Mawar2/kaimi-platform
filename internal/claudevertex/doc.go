// Package claudevertex provides a production Generator backed by Anthropic's
// Claude models served through Google Vertex AI (publisher "anthropic").
//
// It is the shared model client for the Zone 2 agents that run on Claude — the
// Writer ("Thomas", claude-opus-4-8) and the Final Review compliance verifier
// (claude-fable-5). Both consume the Writer's Generator interface
// (GenerateSection(ctx, systemInstruction, prompt) (string, error)); this type
// satisfies it with a single Vertex rawPredict call per invocation, so one
// client serves both agents.
//
// Why Vertex and not the first-party Anthropic API: Kaimi runs entirely on the
// configured GCP project (the GCP_PROJECT_ID set for the deployment) with
// Application Default Credentials and keeps the "built on Google" story. Claude on Vertex uses the same ADC and project as
// Gemini — no Anthropic API key. Region us-east5 hosts the Anthropic publisher
// models. This mirrors how scorer/writer call Gemini directly rather than
// pulling in a full vendor SDK (anti-bloat: minimal dependencies).
package claudevertex
