package dashboard

import (
	_ "embed"
	"encoding/base64"
	"fmt"
)

// This file self-hosts the two designed typefaces so the dashboard renders in
// Figtree (sans) and Geist Mono (mono) on every machine, instead of silently
// falling back to system fonts when those faces are not installed locally. The
// design system names both as the primary families (--font-sans / --font-mono
// in tokens.css), and the comps were drawn in them; without self-hosting, the
// served UI drifts from the handoff on any clean machine or the deployed app.
//
// Delivery rule (docs/dashboard/ux-spec.md): no external assets are ever
// fetched. The faces are embedded as inline base64 data-URIs, so a page is one
// self-contained document with zero network font requests.
//
// Both faces are variable fonts (weight axis), licensed under the SIL Open Font
// License 1.1 (see fonts/Figtree-OFL.txt, fonts/GeistMono-OFL.txt). Using the
// variable build is deliberate: the type tokens ask for non-standard weights
// (420/430/550/650) that static cuts cannot supply. The bytes are the latin
// `wght` subset from the @fontsource-variable packages.

//go:embed fonts/figtree-variable.woff2
var figtreeWOFF2 []byte

//go:embed fonts/geist-mono-variable.woff2
var geistMonoWOFF2 []byte

// fontFaceCSS is the @font-face block prepended to StyleTag's stylesheet. It is
// computed once at package init (base64 of ~50KB) rather than per request.
var fontFaceCSS = embeddedFontFace("Figtree", figtreeWOFF2) +
	embeddedFontFace("Geist Mono", geistMonoWOFF2)

// embeddedFontFace renders one @font-face rule that inlines a variable woff2 as
// a base64 data-URI. The weight range 100 900 exposes the full variable axis so
// any weight the type tokens request resolves to the real face, not a synthetic
// bold. font-display:swap keeps text visible during the (instant, inline) load.
func embeddedFontFace(family string, woff2 []byte) string {
	enc := base64.StdEncoding.EncodeToString(woff2)
	return fmt.Sprintf(
		`@font-face{font-family:%q;font-style:normal;font-weight:100 900;`+
			`font-display:swap;src:url(data:font/woff2;base64,%s) format("woff2");}`,
		family, enc,
	)
}
