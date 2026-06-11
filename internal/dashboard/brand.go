package dashboard

import (
	"fmt"
	"html/template"
)

// This file implements the locked Kaimi brand system ("Kai wave") from the
// design handoff (Kaimi Brand.html). The mark is a cyan rising sun over two
// waves — Kai is Hawaiian for sea; the sun is the seeker, scanning the
// horizon. Cyan leads, navy grounds.
//
// Brand usage rules (encoded here, do not bypass at call sites):
//   - Keep clear space of at least the sun's diameter on every side.
//   - Minimum mark size is 20 px digital; Mark clamps smaller requests.
//   - Below 24 px the back wave is dropped and strokes thicken so the mark
//     stays crisp (the "single-wave fallback"); Mark applies this automatically.
//   - Never recolor the sun, rotate the mark, or add effects.

// Brand palette colors. These are the only colors the brand mark uses; the
// dashboard's shared layout (docs/dashboard/ux-spec.md) maps its accents to
// the same values so the UI and the mark stay coherent.
const (
	// BrandNavy is the grounding ink navy used for text and the back wave.
	BrandNavy = "#0A1B3D"
	// BrandBlue is the Kaimi house blue, the start of the front-wave gradient.
	BrandBlue = "#2563EB"
	// BrandCyan is the Kaimi cyan accent — the color of the sun.
	BrandCyan = "#22D3EE"
	// BrandCyanDark is the darker cyan variant, the end of the front-wave
	// gradient and the eyebrow/accent color on light backgrounds.
	BrandCyanDark = "#0EA5C4"
)

// MarkVariant selects a color treatment of the Kai wave mark.
type MarkVariant string

const (
	// MarkPrimary is the full-color mark for light backgrounds: cyan sun,
	// blue-to-cyan gradient front wave, navy back wave.
	MarkPrimary MarkVariant = "primary"
	// MarkReversed is the lockup for navy / dark surfaces: cyan sun, light
	// blue front wave, white back wave.
	MarkReversed MarkVariant = "reversed"
	// MarkMonoNavy is the one-color navy mark for print and single-color
	// contexts on light backgrounds.
	MarkMonoNavy MarkVariant = "mono-navy"
	// MarkMonoWhite is the one-color white mark for single-color contexts on
	// dark backgrounds.
	MarkMonoWhite MarkVariant = "mono-white"
)

const (
	// markMinSizePx is the brand's minimum digital mark size.
	markMinSizePx = 20
	// markSingleWaveBelowPx is the size under which the single-wave fallback
	// applies: the back wave is dropped and strokes thicken.
	markSingleWaveBelowPx = 24
)

// SVG geometry from Kaimi Brand.html. All marks share a 64x64 viewBox and
// scale via the width/height attributes.
const (
	markFrontWavePath = "M9 37.5C16.5 28 23.5 28 31 37.5C38.5 47 45.5 47 53 37.5"
	markBackWavePath  = "M9 47.5C16.5 38 23.5 38 31 47.5C38.5 57 45.5 57 53 47.5"
	// markSmallWavePath is the thicker single wave used below 24 px.
	markSmallWavePath = "M9 38C17 28 24 28 31 38C38 48 45 48 53 38"
)

// markPalette holds the per-variant colors of the mark's three elements.
type markPalette struct {
	sun         string
	front       string // empty means "use the blue-to-cyan brand gradient"
	back        string // also the solid wave color in the single-wave fallback
	backOpacity string
}

var markPalettes = map[MarkVariant]markPalette{
	MarkPrimary:   {sun: BrandCyan, front: "", back: BrandNavy, backOpacity: "0.9"},
	MarkReversed:  {sun: BrandCyan, front: "#5B9BFF", back: "#FFFFFF", backOpacity: "0.92"},
	MarkMonoNavy:  {sun: BrandNavy, front: BrandNavy, back: BrandNavy, backOpacity: "0.5"},
	MarkMonoWhite: {sun: "#FFFFFF", front: "#FFFFFF", back: "#FFFFFF", backOpacity: "0.55"},
}

// Mark renders the Kai wave mark as a self-contained inline SVG at sizePx
// pixels square. Unknown variants fall back to MarkPrimary. Sizes below the
// 20 px brand minimum are clamped; below 24 px the single-wave fallback is
// used automatically.
//
// The primary variant embeds its own gradient definition, so the returned SVG
// has no external dependencies. If the same size of the primary mark appears
// more than once on a page the duplicated gradient id is harmless — browsers
// resolve the reference to the first definition, which is identical.
//
// The SVG is decorative (aria-hidden); place visible text such as the
// wordmark next to it, as HeaderLockup does.
func Mark(variant MarkVariant, sizePx int) template.HTML {
	if sizePx < markMinSizePx {
		sizePx = markMinSizePx
	}
	pal, ok := markPalettes[variant]
	if !ok {
		pal = markPalettes[MarkPrimary]
	}

	if sizePx < markSingleWaveBelowPx {
		// Single-wave fallback: sun plus one solid, thicker wave.
		// #nosec G203 -- built entirely from package-level constants.
		return template.HTML(fmt.Sprintf(
			`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 64 64" fill="none" aria-hidden="true">`+
				`<circle cx="45" cy="19" r="8" fill=%q/>`+
				`<path d=%q stroke=%q stroke-width="6.5" stroke-linecap="round"/>`+
				`</svg>`,
			sizePx, sizePx, pal.sun, markSmallWavePath, pal.back))
	}

	front := pal.front
	defs := ""
	if front == "" {
		// The gradient id carries the size so different sizes on one page
		// never collide.
		gradID := fmt.Sprintf("kaimiKW%d", sizePx)
		defs = fmt.Sprintf(
			`<defs><linearGradient id=%q x1="9" y1="32" x2="53" y2="32" gradientUnits="userSpaceOnUse">`+
				`<stop offset="0" stop-color=%q/><stop offset="1" stop-color=%q/>`+
				`</linearGradient></defs>`,
			gradID, BrandBlue, BrandCyanDark)
		front = fmt.Sprintf("url(#%s)", gradID)
	}

	// #nosec G203 -- built entirely from package-level constants.
	return template.HTML(fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 64 64" fill="none" aria-hidden="true">`+
			`%s`+
			`<circle cx="45" cy="18.5" r="6.6" fill=%q/>`+
			`<path d=%q stroke=%q stroke-width="4.8" stroke-linecap="round"/>`+
			`<path d=%q stroke=%q stroke-width="4.8" stroke-linecap="round" opacity=%q/>`+
			`</svg>`,
		sizePx, sizePx, defs, pal.sun, markFrontWavePath, front, markBackWavePath, pal.back, pal.backOpacity))
}

// faviconDataURI is the brand favicon verbatim from Kaimi Brand.html: a
// rounded navy square with the cyan sun and a single white wave (the
// single-wave fallback, since favicons render at 16-32 px).
const faviconDataURI = "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'%3E%3Crect width='64' height='64' rx='14' fill='%230A1B3D'/%3E%3Ccircle cx='44' cy='21' r='8' fill='%2322D3EE'/%3E%3Cpath d='M10 40C18 30 25 30 32 40C39 50 46 50 54 40' stroke='white' stroke-width='6.5' fill='none' stroke-linecap='round'/%3E%3C/svg%3E"

// FaviconLink renders the <link> tag for the Kaimi favicon as an inline data
// URI, so the dashboard ships the brand icon with no external assets.
func FaviconLink() template.HTML {
	// #nosec G203 -- constant markup, no user input.
	return template.HTML(`<link rel="icon" href="` + faviconDataURI + `"/>`)
}

// HeaderLockup renders the compact horizontal brand lockup for the dashboard
// header: the primary mark beside the "Kaimi" wordmark and the "THE SEEKER"
// sub-label. Styling is inline CSS only, per the ux-spec technology
// constraints.
func HeaderLockup() template.HTML {
	// #nosec G203 -- constant markup, no user input.
	return template.HTML(fmt.Sprintf(
		`<div style="display:flex;align-items:center;gap:10px;margin:0 0 1rem 0">`+
			`%s`+
			`<span style="font-weight:800;font-size:20px;letter-spacing:-0.03em;color:%s">Kaimi</span>`+
			`<span style="font-weight:600;font-size:11px;letter-spacing:0.1em;color:#94A3BE">THE SEEKER</span>`+
			`</div>`,
		Mark(MarkPrimary, 28), BrandNavy))
}
