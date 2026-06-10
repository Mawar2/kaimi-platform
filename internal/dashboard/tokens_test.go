package dashboard

import (
	"strings"
	"testing"
)

func TestStyleTagIsAStyleElement(t *testing.T) {
	got := string(StyleTag())
	if !strings.HasPrefix(got, "<style>") || !strings.HasSuffix(got, "</style>") {
		t.Fatalf("StyleTag must emit a single <style> element, got prefix %q / suffix %q",
			got[:min(20, len(got))], got[max(0, len(got)-20):])
	}
}

func TestStyleTagContainsDesignTokens(t *testing.T) {
	got := string(StyleTag())
	// One representative token per token group from kaimi/tokens.css.
	wants := []string{
		"--blue-900: #0A1B3D", // house navy ink
		"--cyan-400: #22D3EE", // Kaimi accent
		"--n-400: #94A3BE",    // navy-tinted neutral
		"--st-human:",         // needs-human amber (the loudest signal)
		"#E8870E",
		"--st-progress:",
		"--rec-nobid:",
		"--fit-strong:",
		"--fit-track:",
		"--urg-crit:",
		"--t-h1:",
		"--s-7: 32px",
		"--r-pill: 999px",
		"--e-4:",
		"--m-slow: 360ms",
		"--ease-spring:",
		`[data-theme="focus"]`, // dark Focus theme ships with the tokens
		"prefers-reduced-motion",
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("StyleTag() missing token %q", want)
		}
	}
}

func TestStyleTagContainsComponentClasses(t *testing.T) {
	got := string(StyleTag())
	// One selector per component family from kaimi/ui.css.
	wants := []string{
		".kbadge--human",
		".kbadge--progress .dot",
		".krec--review",
		".kfit",
		".kfit-num",
		".kdead--crit",
		".kbtn--select",
		".kbtn--approve",
		".kbtn--changes",
		".kava",
		".kchip--on",
		".ktag",
		"@keyframes kHumanPulse",
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("StyleTag() missing component rule %q", want)
		}
	}
}

func TestStyleTagLoadsNoExternalAssets(t *testing.T) {
	got := string(StyleTag())
	for _, ban := range []string{"@import", "url(http", "fonts.googleapis"} {
		if strings.Contains(got, ban) {
			t.Errorf("StyleTag must not load external assets, found %q", ban)
		}
	}
}

// TestStyleTagSelfHostsDesignedFonts proves the designed faces actually render
// instead of falling back to system fonts: the design system declares Figtree
// (sans) and Geist Mono (mono) as the primary families, so StyleTag must embed
// each as an inline @font-face data-URI (no network fetch — see the
// no-external-assets test above). The fonts are variable, so each face declares
// the full weight range to cover the non-standard token weights (420/430/550/650).
func TestStyleTagSelfHostsDesignedFonts(t *testing.T) {
	got := string(StyleTag())
	wants := []string{
		"@font-face",                      // the faces are embedded, not assumed installed
		`font-family:"Figtree"`,           // sans face self-hosted
		`font-family:"Geist Mono"`,        // mono face self-hosted (design-system primary)
		"font-weight:100 900",             // variable axis covers 420/430/550/650
		"src:url(data:font/woff2;base64,", // inline data-URI, no external request
		`format("woff2")`,
		`--font-sans: "Figtree"`,    // token still names Figtree first
		`--font-mono: "Geist Mono"`, // token still names Geist Mono first
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("StyleTag() must self-host designed fonts: missing %q", want)
		}
	}
	// Exactly two embedded faces (Figtree + Geist Mono), no accidental extras.
	if n := strings.Count(got, "@font-face"); n != 2 {
		t.Errorf("StyleTag() should embed exactly 2 @font-face faces, found %d", n)
	}
}

func TestStyleTagContainsAppShellStyles(t *testing.T) {
	got := string(StyleTag())
	// One selector per app.css family (issue #150): shell, sidebar, page,
	// stats, toolbar, opportunity list, drawer content.
	wants := []string{
		"--sidebar: 248px",
		".app{",
		".side{",
		".nav-item.on{",
		".page-head",
		".stats{",
		".seg button.on{",
		".sortbtn",
		".opp-list .day",
		".orow",
		".rec-min--bid",
		".empty2",
		".dr-top",
		".reasons li",
		".must.ok .mc",
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("StyleTag() missing app style %q", want)
		}
	}
}
