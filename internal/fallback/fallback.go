// Package fallback adds production-grade resilience to the agents' model-call
// interfaces by failing over across real model backends.
//
// Each agent (Writer, Final Review, Outline) reaches its model through a
// single-method interface. This package wraps a primary implementation with one
// or more backups: each option is retried a bounded number of times on transient
// errors (throttling, 5xx, timeouts) before failing over to the next. The first
// success wins.
//
// When every real-model option is exhausted it returns the last error — it NEVER
// substitutes a stub or fabricated result. The agent's own honest degrade then applies
// (the Writer returns a failed status behind the human gate; the Final Review routes to
// needs-human while its deterministic checks still stand; the Outline reports a failed
// status and the chain halts). This package imports only the agent interfaces, not
// their internals, so it does not collide with in-flight agent work.
package fallback

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/Mawar2/Kaimi/internal/finalreview"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/outline"
	"github.com/Mawar2/Kaimi/internal/writer"
)

// RetryBackoff is the pause between retry attempts on a single option after a transient
// error. Exposed so tests can set it to zero; defaults to a production-sane value.
var RetryBackoff = 750 * time.Millisecond

// maxAttemptsPerOption bounds same-option retries before failing over to the next backend.
const maxAttemptsPerOption = 2

// run tries each option in order, retrying transient failures on the same option before
// moving on. It returns the first success, or the last error if every option fails.
// The options close over their call arguments, so any result type works — the Writer
// and Final Review return text, the Outline planner returns sections.
func run[T any](ctx context.Context, label string, options []func(context.Context) (T, error)) (T, error) {
	var zero T
	var lastErr error
	for i, c := range options {
		for attempt := 1; attempt <= maxAttemptsPerOption; attempt++ {
			out, err := c(ctx)
			if err == nil {
				if i > 0 || attempt > 1 {
					log.Printf("[fallback:%s] recovered on option #%d (attempt %d)", label, i, attempt)
				}
				return out, nil
			}
			lastErr = err
			log.Printf("[fallback:%s] option #%d attempt %d failed: %v", label, i, attempt, err)

			// Retry the same option only for transient errors and only if attempts remain.
			if attempt < maxAttemptsPerOption && isTransient(err) && ctx.Err() == nil {
				select {
				case <-ctx.Done():
					return zero, ctx.Err()
				case <-time.After(RetryBackoff):
				}
				continue
			}
			break // non-transient, or out of attempts: fail over to the next option
		}
	}
	return zero, lastErr
}

// isTransient reports whether an error looks retryable (throttling, 5xx, timeouts).
// It matches on the error text because the Vertex SDK surfaces these as wrapped errors.
func isTransient(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	for _, k := range []string{
		"429", "resource_exhausted", "rate limit", "quota",
		"503", "500", "502", "504", "unavailable", "internal error",
		"deadline", "timeout", "temporar", "connection reset", "eof",
	} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// --- Writer.Generator ---

type generator struct{ options []writer.Generator }

// NewGenerator wraps a primary writer.Generator with ordered real-model backups. The
// returned Generator tries primary first, then each backup, on failure — no stub.
func NewGenerator(primary writer.Generator, backups ...writer.Generator) writer.Generator {
	return &generator{options: append([]writer.Generator{primary}, backups...)}
}

func (g *generator) GenerateSection(ctx context.Context, systemInstruction, prompt string) (string, error) {
	options := make([]func(context.Context) (string, error), len(g.options))
	for i := range g.options {
		gen := g.options[i]
		options[i] = func(ctx context.Context) (string, error) {
			return gen.GenerateSection(ctx, systemInstruction, prompt)
		}
	}
	return run(ctx, "writer", options)
}

// --- finalreview.ComplianceChecker ---

type checker struct {
	options []finalreview.ComplianceChecker
}

// NewChecker wraps a primary finalreview.ComplianceChecker with ordered real-model
// backups. The returned checker fails over across backends; if all fail, the Final
// Review agent's own needs-human degrade applies.
func NewChecker(primary finalreview.ComplianceChecker, backups ...finalreview.ComplianceChecker) finalreview.ComplianceChecker {
	return &checker{options: append([]finalreview.ComplianceChecker{primary}, backups...)}
}

func (c *checker) CheckCompliance(ctx context.Context, systemInstruction, prompt string) (string, error) {
	options := make([]func(context.Context) (string, error), len(c.options))
	for i := range c.options {
		ck := c.options[i]
		options[i] = func(ctx context.Context) (string, error) {
			return ck.CheckCompliance(ctx, systemInstruction, prompt)
		}
	}
	return run(ctx, "final-review", options)
}

// --- outline.SectionPlanner ---

type planner struct{ options []outline.SectionPlanner }

// NewPlanner wraps a primary outline.SectionPlanner with ordered real-model backups.
// The returned planner fails over across backends; if all fail, the Outline agent
// reports a failed status and the proposal chain halts honestly — no stub sections
// (issue #266: Outline is the first agent every proposal hits, so a single transient
// model error must not kill the chain).
func NewPlanner(primary outline.SectionPlanner, backups ...outline.SectionPlanner) outline.SectionPlanner {
	return &planner{options: append([]outline.SectionPlanner{primary}, backups...)}
}

func (p *planner) PlanSections(ctx context.Context, opp *opportunity.Opportunity, source string) ([]outline.Section, error) {
	options := make([]func(context.Context) ([]outline.Section, error), len(p.options))
	for i := range p.options {
		pl := p.options[i]
		options[i] = func(ctx context.Context) ([]outline.Section, error) {
			return pl.PlanSections(ctx, opp, source)
		}
	}
	return run(ctx, "outline", options)
}
