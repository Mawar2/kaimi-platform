package dashboard_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/store"
)

func TestHandleList(t *testing.T) {
	ctx := context.Background()
	s, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)

	// Seed some opportunities
	opps := []*opportunity.Opportunity{
		{
			ID:               "soon",
			Title:            "Deadline Soon",
			Agency:           "Agency A",
			Score:            0.9,
			ScoredAt:         &now,
			ResponseDeadline: now.Add(2 * 24 * time.Hour),
			UpdatedAt:        now,
		},
		{
			ID:               "late",
			Title:            "Deadline Late",
			Agency:           "Agency B",
			Score:            0.5,
			ScoredAt:         &now,
			ResponseDeadline: now.Add(10 * 24 * time.Hour),
			UpdatedAt:        now,
		},
		{
			ID:        "hunted",
			Title:     "Not Scored",
			Agency:    "Agency C",
			Score:     0,
			ScoredAt:  nil,
			UpdatedAt: now,
		},
	}
	for _, opp := range opps {
		if err := s.Save(ctx, opp); err != nil {
			t.Fatalf("failed to seed opportunity: %v", err)
		}
	}

	svc := dashboard.NewService(s)
	// We'll define NewHandler in handler.go
	h := dashboard.NewHandler(svc)
	h.Now = func() time.Time { return now }

	tests := []struct {
		name          string
		query         string
		wantStatus    int
		containsTexts []string
		excludesTexts []string
	}{
		{
			name:       "default list",
			query:      "",
			wantStatus: http.StatusOK,
			containsTexts: []string{
				"Deadline Soon",
				"Deadline Late",
				"Not Scored",
				"kdead--crit", // deadline pill at 2 days (designed urgency treatment)
			},
		},
		{
			name:       "filter by stage",
			query:      "?stage=Scored",
			wantStatus: http.StatusOK,
			containsTexts: []string{
				"Deadline Soon",
				"Deadline Late",
			},
			excludesTexts: []string{
				"Not Scored",
			},
		},
		{
			name:       "filter by minScore",
			query:      "?minScore=0.8",
			wantStatus: http.StatusOK,
			containsTexts: []string{
				"Deadline Soon",
			},
			excludesTexts: []string{
				"Deadline Late",
				"Not Scored",
			},
		},
		{
			name:       "sort by score",
			query:      "?sort=score",
			wantStatus: http.StatusOK,
			containsTexts: []string{
				"Deadline Soon",
				"Deadline Late",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/"+tc.query, http.NoBody)
			rr := httptest.NewRecorder()

			h.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("got status %v, want %v", rr.Code, tc.wantStatus)
			}

			body := rr.Body.String()
			for _, text := range tc.containsTexts {
				if !contains(body, text) {
					t.Errorf("body missing expected text %q", text)
				}
			}
			for _, text := range tc.excludesTexts {
				if contains(body, text) {
					t.Errorf("body contains unexpected text %q", text)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return (len(s) >= len(substr)) && (func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}

// TestHandleListAdoptsDesignSystem verifies the overview layout consumes the
// brand and design-system assets (GitHub issue #141) instead of the
// pre-handoff placeholder styling.
func TestHandleListAdoptsDesignSystem(t *testing.T) {
	ctx := context.Background()
	s, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	if err := s.Save(ctx, &opportunity.Opportunity{
		ID: "one", Title: "Sample", Agency: "Agency", UpdatedAt: now,
	}); err != nil {
		t.Fatalf("failed to seed opportunity: %v", err)
	}

	h := dashboard.NewHandler(dashboard.NewService(s))
	h.Now = func() time.Time { return now }

	req := httptest.NewRequest("GET", "/", http.NoBody)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("got status %v, want %v", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()

	for _, want := range []string{
		`rel="icon"`,         // brand favicon (FaviconLink)
		"--st-human:",        // design-system tokens present (StyleTag)
		"#E8870E",            // the needs-human amber is defined
		"the seeker",         // header lockup (HeaderLockup)
		"var(--st-human-bg)", // page styles expressed in token variables
	} {
		if !contains(body, want) {
			t.Errorf("overview body missing design-system marker %q", want)
		}
	}

	for _, ban := range []string{
		"#fffbe6", "#f0c040", "#0057b8", "#fff0f0", // placeholder palette
		"<h1>Kaimi Pipeline</h1>", // replaced by the lockup
	} {
		if contains(body, ban) {
			t.Errorf("overview body still contains placeholder styling %q", ban)
		}
	}
}

// seedDetailOpp is a fully-populated opportunity for detail-page tests.
func seedDetailOpp(t *testing.T, s store.Store, now time.Time) *opportunity.Opportunity {
	t.Helper()
	opp := &opportunity.Opportunity{
		ID:                 "ztamod-001",
		Title:              "Zero Trust Architecture Modernization",
		SolicitationNum:    "70RCSA24R0123",
		Agency:             "Dept. of Homeland Security",
		Office:             "CISA",
		PostedDate:         now.Add(-10 * 24 * time.Hour),
		ResponseDeadline:   now.Add(9 * 24 * time.Hour),
		NAICSCode:          "541512",
		NAICSDescription:   "Computer Systems Design Services",
		SetAsideCode:       "SBA",
		PlaceOfPerformance: "Washington, DC",
		Description:        "Modernize the agency's zero trust architecture.",
		Type:               "Solicitation",
		ContractType:       "Firm Fixed Price",
		URL:                "https://sam.gov/opp/ztamod-001",
		Score:              0.82,
		ScoreReasoning:     "Strong past performance in cybersecurity.",
		Recommendation:     "BID",
		Requirements:       []string{"FedRAMP High", "Top Secret facility clearance"},
		ScoredAt:           &now,
		CreatedAt:          now.Add(-10 * 24 * time.Hour),
		UpdatedAt:          now,
	}
	if err := s.Save(context.Background(), opp); err != nil {
		t.Fatalf("failed to seed opportunity: %v", err)
	}
	return opp
}

// TestHandleDetail verifies the /opportunity/{id} page (GitHub issue #111).
func TestHandleDetail(t *testing.T) {
	s, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	seedDetailOpp(t, s, now)

	h := dashboard.NewHandler(dashboard.NewService(s))
	h.Now = func() time.Time { return now }

	t.Run("valid id renders the full record", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", "/opportunity/ztamod-001", http.NoBody))
		if rr.Code != http.StatusOK {
			t.Fatalf("got status %v, want %v", rr.Code, http.StatusOK)
		}
		body := rr.Body.String()
		for _, want := range []string{
			"Zero Trust Architecture Modernization",
			"Dept. of Homeland Security",
			"70RCSA24R0123",
			"Computer Systems Design Services",
			"Modernize the agency&#39;s zero trust architecture.",
			"Strong past performance in cybersecurity.",
			"FedRAMP High",
			"82.0%",                               // ScoreDisplay
			`class="kfit"`,                        // FitRing (design system)
			"krec--bid",                           // RecommendationPill
			"kdead--near",                         // DeadlinePill at 9 days
			`class="ktag"`,                        // MetaTag for NAICS/SOL
			`id="eligibility-note"`,               // Zone-1 gate note (issue #256)
			"Passed Zone-1 eligibility screening", // honest copy — the gate IS implemented
			"All opportunities",                   // back link
			"View solicitation",                   // solicitation link
			"Scored",                              // derived stage
			`http-equiv="refresh"`,                // live page keeps auto-refresh
			"the seeker",                          // shared branded layout
		} {
			if !contains(body, want) {
				t.Errorf("detail body missing %q", want)
			}
		}
	})

	t.Run("unknown id returns 404 without refresh", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", "/opportunity/nope", http.NoBody))
		if rr.Code != http.StatusNotFound {
			t.Fatalf("got status %v, want %v", rr.Code, http.StatusNotFound)
		}
		body := rr.Body.String()
		if !contains(body, "Opportunity not found: nope") {
			t.Errorf("404 body missing not-found message, got:\n%s", body)
		}
		if contains(body, `http-equiv="refresh"`) {
			t.Errorf("404 page must not auto-refresh (ux-spec)")
		}
	})

	t.Run("invalid id characters are rejected with 404", func(t *testing.T) {
		for _, id := range []string{"bad%24id", "a%20b", "%2e%2e", "x%3Cscript%3E"} {
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, httptest.NewRequest("GET", "/opportunity/"+id, http.NoBody))
			if rr.Code != http.StatusNotFound {
				t.Errorf("id %q: got status %v, want 404", id, rr.Code)
			}
			if contains(rr.Body.String(), "<script>") {
				t.Errorf("id %q: unescaped input reflected in response", id)
			}
		}
	})

	t.Run("unscored opportunity shows dashes and no ring", func(t *testing.T) {
		if err := s.Save(context.Background(), &opportunity.Opportunity{
			ID: "raw-1", Title: "Unscored Opp", Agency: "GSA", UpdatedAt: now,
		}); err != nil {
			t.Fatalf("failed to seed: %v", err)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", "/opportunity/raw-1", http.NoBody))
		if rr.Code != http.StatusOK {
			t.Fatalf("got status %v, want %v", rr.Code, http.StatusOK)
		}
		body := rr.Body.String()
		if contains(body, `class="kfit"`) {
			t.Errorf("unscored detail should not render a fit ring")
		}
		if !contains(body, "Hunted") {
			t.Errorf("unscored detail should show the Hunted stage")
		}
	})
}

// TestDetailRoutesThroughDesignTokens proves the detail surface consumes the
// design system rather than re-hardcoding values per page (issue #205): the
// title typography comes from the shared .dr-top h2 rule (not an inline font),
// and the page-local .kv / .detail-pre table styles use --t-* / --s-* tokens
// instead of magic numbers. Guards the "define once, reuse" rule for this page.
func TestDetailRoutesThroughDesignTokens(t *testing.T) {
	s, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	seedDetailOpp(t, s, now)

	h := dashboard.NewHandler(dashboard.NewService(s))
	h.Now = func() time.Time { return now }
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/opportunity/ztamod-001", http.NoBody))
	body := rr.Body.String()

	// The title must still render inside the drawer header, but its typography
	// must come from .dr-top h2 — no inline font/letter-spacing re-hardcode.
	for _, want := range []string{`class="dr-top"`, "<h2>"} {
		if !contains(body, want) {
			t.Errorf("detail body missing %q (title should render via .dr-top h2)", want)
		}
	}
	// These literals are the per-page re-hardcodes this change removes.
	for _, banned := range []string{
		"<h2 style=",        // title must carry no inline typography (use .dr-top h2)
		"font-size: 13.5px", // .kv magic number
		"0.4rem 0.7rem",     // .kv padding magic numbers
		"padding: 0.75rem",  // .detail-pre padding magic number
	} {
		if contains(body, banned) {
			t.Errorf("detail page must not re-hardcode %q; route it through a token", banned)
		}
	}
	// The page-local table styles must reference the token vocabulary.
	for _, want := range []string{"var(--t-small)", "var(--s-"} {
		if !contains(body, want) {
			t.Errorf("detail page-local styles must use design tokens; missing %q", want)
		}
	}
}

// TestListLinksToDetail verifies table rows link to their detail page (#111).
func TestListLinksToDetail(t *testing.T) {
	s, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	seedDetailOpp(t, s, now)

	h := dashboard.NewHandler(dashboard.NewService(s))
	h.Now = func() time.Time { return now }

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/", http.NoBody))
	if !contains(rr.Body.String(), `href="/opportunity/ztamod-001"`) {
		t.Errorf("list table should link rows to their detail page")
	}
}

// TestTriageScreen verifies the designed Opportunities app surface
// (GitHub issue #150; visual source: design handoff Kaimi App.html).
func TestTriageScreen(t *testing.T) {
	s, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	seedDetailOpp(t, s, now) // scored BID, created 10 days ago
	if err := s.Save(context.Background(), &opportunity.Opportunity{
		ID: "fresh-1", Title: "Fresh Today Opp", Agency: "GSA",
		Recommendation: "REVIEW", Score: 0.55, ScoredAt: &now,
		CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	h := dashboard.NewHandler(dashboard.NewService(s))
	h.Now = func() time.Time { return now }

	t.Run("app shell and triage furniture", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", "/", http.NoBody))
		body := rr.Body.String()
		for _, want := range []string{
			`class="app"`,         // shell grid
			`class="side"`,        // sidebar
			"the seeker",          // lockup sub-label
			"Pipeline",            // nav section label
			">Triage<",            // page-head eyebrow
			">Opportunities</h1>", // H1
			"in queue",            // stat strip
			"Added today",         // stat strip
			"Top fit score",       // stat strip
			`class="seg"`,         // segmented filter
			"To pursue",           // segment labels
			"Needs review",
			"New today",        // day group for fresh-1
			`class="orow new"`, // new-dot row variant
			`class="kfit"`,     // FitRing in rows
			"rec-min--bid",     // recommendation word variants
			"rec-min--review",
			"Fresh Today Opp",
			"Zero Trust Architecture Modernization",
		} {
			if !contains(body, want) {
				t.Errorf("triage screen missing %q", want)
			}
		}
		if contains(body, "<table") {
			t.Errorf("triage screen must render row cards, not the old table")
		}
	})

	t.Run("segments filter by recommendation", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", "/?rec=REVIEW", http.NoBody))
		body := rr.Body.String()
		if !contains(body, "Fresh Today Opp") || contains(body, "Zero Trust Architecture Modernization") {
			t.Errorf("rec=REVIEW should keep only REVIEW rows")
		}
	})

	t.Run("empty state when nothing matches", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", "/?rec=NO_BID", http.NoBody))
		if !contains(rr.Body.String(), "empty2") {
			t.Errorf("no matches should render the designed empty state")
		}
	})

	t.Run("detail page carries drawer-style header", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", "/opportunity/ztamod-001", http.NoBody))
		body := rr.Body.String()
		for _, want := range []string{
			`class="app"`,            // detail lives in the shell too
			`class="dr-top"`,         // drawer-style top block
			">FIT<",                  // 92px ring sublabel
			"Why Kaimi scored this",  // reasons section (CSS uppercases)
			"Must-have requirements", // checklist section (CSS uppercases)
			`class="must ok"`,        // checklist item
			"View solicitation",      // ghost link
			`id="eligibility-note"`,  // ux-spec field retained (issue #256 copy)
		} {
			if !contains(body, want) {
				t.Errorf("detail missing %q", want)
			}
		}
	})
}

// TestSidebarHasSubmittedNav verifies the new third sidebar nav item (the
// Submitted archive) renders on the shared app shell with its archive icon and
// active-state wiring (new Kaimi App.html design).
func TestSidebarHasSubmittedNav(t *testing.T) {
	s, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	if err := s.Save(context.Background(), &opportunity.Opportunity{
		ID: "o1", Title: "Sample", Agency: "GSA", UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	h := dashboard.NewHandler(dashboard.NewService(s))
	h.Now = func() time.Time { return now }
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/", http.NoBody))
	body := rr.Body.String()
	if !contains(body, `href="/submitted"`) || !contains(body, "<span>Submitted</span>") {
		t.Errorf("shell sidebar must render the Submitted nav item, got:\n%s", body[:min(800, len(body))])
	}
	// The template action must be rendered, not emitted literally.
	if contains(body, "ActiveNav") {
		t.Errorf("unrendered template action leaked into output")
	}
}

// TestSidebarTenantName verifies the sidebar account block renders the
// configured tenant display name (and its derived initials) instead of any
// hardcoded customer identity (WS-A5). An empty/absent name must fall back to
// the neutral "Kaimi" product label and never render blank.
func TestSidebarTenantName(t *testing.T) {
	render := func(opts ...dashboard.Option) string {
		s, err := store.NewJSONStore(t.TempDir())
		if err != nil {
			t.Fatalf("store: %v", err)
		}
		now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
		if err := s.Save(context.Background(), &opportunity.Opportunity{
			ID: "o1", Title: "Sample", Agency: "GSA", UpdatedAt: now,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
		h := dashboard.NewHandler(dashboard.NewService(s), opts...)
		h.Now = func() time.Time { return now }
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", "/", http.NoBody))
		return rr.Body.String()
	}

	t.Run("configured name renders", func(t *testing.T) {
		body := render(dashboard.WithTenantName("Acme Federal Co"))
		if !contains(body, "Acme Federal Co") {
			t.Errorf("sidebar must render the configured tenant name; got:\n%s", body[:min(1200, len(body))])
		}
		// Initials derive from the first two words.
		if !contains(body, `class="av">AF<`) {
			t.Errorf("sidebar avatar must show derived initials AF; got:\n%s", body[:min(1200, len(body))])
		}
		// The old hardcoded customer identity must be gone.
		if contains(body, "BlueMeta") {
			t.Errorf("sidebar must not hardcode BlueMeta; got:\n%s", body[:min(1200, len(body))])
		}
	})

	t.Run("multibyte name renders rune-safe initials", func(t *testing.T) {
		// Accented, multibyte display name: byte-slicing the first rune would
		// split "É" mid-codepoint and emit U+FFFD. Initials must be the first
		// rune of each of the first two words, upper-cased ("ÉF"), and valid
		// UTF-8 with no replacement character.
		body := render(dashboard.WithTenantName("Équipe Fédérale"))
		if !contains(body, "Équipe Fédérale") {
			t.Errorf("sidebar must render the multibyte tenant name; got:\n%s", body[:min(1200, len(body))])
		}
		if !contains(body, `class="av">ÉF<`) {
			t.Errorf("sidebar avatar must show rune-safe initials ÉF; got:\n%s", body[:min(1200, len(body))])
		}
		if !utf8.ValidString(body) {
			t.Error("rendered sidebar must be valid UTF-8 (rune-safe initials)")
		}
		if contains(body, "�") {
			t.Errorf("sidebar must not contain the U+FFFD replacement char; got:\n%s", body[:min(1200, len(body))])
		}
	})

	t.Run("empty name falls back to product label", func(t *testing.T) {
		body := render() // no WithTenantName
		if !contains(body, "Kaimi") {
			t.Errorf("empty tenant must fall back to the Kaimi product label; got:\n%s", body[:min(1200, len(body))])
		}
		if contains(body, "BlueMeta") {
			t.Errorf("sidebar must not hardcode BlueMeta; got:\n%s", body[:min(1200, len(body))])
		}
		// Must never render a blank account name.
		if contains(body, `<b></b>`) {
			t.Errorf("sidebar account name must never render blank; got:\n%s", body[:min(1200, len(body))])
		}
	})
}
