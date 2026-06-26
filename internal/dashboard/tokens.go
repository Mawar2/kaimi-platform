package dashboard

import "html/template"

// This file embeds the Kaimi design tokens and shared component styles from
// the design handoff (Kaimi Design System.html, kaimi/tokens.css, kaimi/ui.css
// — GitHub issue #132). The token files are the authoritative source of all
// visual values; pages must consume StyleTag rather than re-hardcoding colors,
// so the whole UI stays on one vocabulary.
//
// Semantic rule (load-bearing, app-wide): amber (--st-human #E8870E family)
// means "a human is needed" and NOTHING else. Never use it decoratively.
//
// Delivery is inline-only per docs/dashboard/ux-spec.md: the styles are
// emitted into each page's <head>; no external CSS files or fonts are fetched.
// The designed faces are self-hosted as inline base64 @font-face data-URIs (see
// fonts.go), so Figtree (sans) and Geist Mono (mono) render on every machine
// rather than falling back to system fonts.

// designTokensCSS is kaimi/tokens.css from the handoff, verbatim: brand ramps,
// the status vocabulary, fit bands, urgency escalation, semantic surfaces for
// the light Triage theme and the dark Focus theme, typography, spacing, radii,
// elevation, and motion.
const designTokensCSS = `
/* ============================================================
   KAIMI — Design Tokens
   "the seeker"
   Two modes: light TRIAGE (:root) · dark FOCUS ([data-theme="focus"])
   ============================================================ */

:root {
  /* ---- Brand ramp (Kaimi house blue) ---- */
  --blue-50:  #EEF4FF;
  --blue-100: #DCE6FF;
  --blue-200: #B9CEFF;
  --blue-300: #8DAEFF;
  --blue-400: #5B86F7;
  --blue-500: #2563EB;   /* brand primary */
  --blue-600: #1D4ED8;
  --blue-700: #1A3FAE;
  --blue-800: #16327F;
  --blue-900: #0A1B3D;   /* deep navy — the house ink */

  /* ---- Kaimi accent (cyan — the seeker's signal) ---- */
  --cyan-50:  #E7FBFF;
  --cyan-200: #A5EEFB;
  --cyan-300: #67E0F4;
  --cyan-400: #22D3EE;   /* accent */
  --cyan-500: #0EA5C4;
  --cyan-600: #0B7E97;

  /* ---- Neutral (cool, navy-tinted greys) ---- */
  --n-0:   #FFFFFF;
  --n-25:  #FAFCFF;
  --n-50:  #F4F7FC;
  --n-100: #ECF1F9;
  --n-200: #DCE4F0;
  --n-300: #C3CFE1;
  --n-400: #94A3BE;
  --n-500: #64748B;
  --n-600: #475569;
  --n-700: #334155;
  --n-800: #1E293B;
  --n-900: #0F1B30;

  /* ============================================================
     STATUS VOCABULARY — the system's most reused colors
     ============================================================ */

  /* Agent / stage states */
  --st-pending:      #64748B;   /* slate — dormant */
  --st-pending-bg:   #EEF1F6;
  --st-progress:     #2563EB;   /* blue — working */
  --st-progress-bg:  #E7EEFF;
  --st-done:         #15A06B;   /* green — complete */
  --st-done-bg:      #E2F6EE;
  --st-human:        #E8870E;   /* AMBER/GOLD — needs you (loudest) */
  --st-human-tint:   #F6A938;
  --st-human-bg:     #FFF3E0;
  --st-human-glow:   rgba(232,135,14,0.45);
  --st-failed:       #DC2626;   /* red — failed */
  --st-failed-bg:    #FCE8E8;

  /* Bid recommendation */
  --rec-bid:     #15A06B;   /* go — green */
  --rec-bid-bg:  #E2F6EE;
  --rec-nobid:   #C2354A;   /* don't — rose-red */
  --rec-nobid-bg:#FBE9EC;
  --rec-review:  #E8870E;   /* human judgment — amber (same family as Needs Human) */
  --rec-review-bg:#FFF3E0;

  /* Fit-score bands */
  --fit-strong: #15A06B;   /* 80-100 */
  --fit-good:   #0EA5C4;   /* 60-79  */
  --fit-fair:   #E8870E;   /* 40-59  */
  --fit-weak:   #C2354A;   /* 0-39   */
  --fit-track:  #E4EAF3;   /* unfilled ring */

  /* Urgency / deadline escalation */
  --urg-calm:    #64748B;   /* >30d */
  --urg-soon:    #2563EB;   /* 14-30d */
  --urg-near:    #E8870E;   /* 7-14d */
  --urg-crit:    #DC2626;   /* <7d / <72h — pulses */
  --urg-crit-bg: #FCE8E8;

  /* ============================================================
     SEMANTIC SURFACES — light TRIAGE
     ============================================================ */
  --bg:          var(--n-50);
  --bg-sunken:   #EDF2F9;
  --surface:     var(--n-0);
  --surface-2:   var(--n-25);
  --surface-3:   var(--n-100);
  --ink:         var(--blue-900);
  --ink-2:       var(--n-600);
  --ink-3:       var(--n-500);
  --ink-4:       var(--n-400);
  --border:      var(--n-200);
  --border-2:    var(--n-100);
  --primary:     var(--blue-500);
  --primary-ink: #FFFFFF;
  --accent:      var(--cyan-500);
  --ring-focus:  rgba(37,99,235,0.35);

  /* ---- Typography ---- */
  --font-sans: "Figtree", ui-sans-serif, system-ui, -apple-system, sans-serif;
  --font-mono: "Geist Mono", "IBM Plex Mono", ui-monospace, "SF Mono", monospace;

  --t-display: 700 48px/1.04 var(--font-sans);
  --t-h1:      700 33px/1.1  var(--font-sans);
  --t-h2:      650 25px/1.18 var(--font-sans);
  --t-h3:      650 19px/1.28 var(--font-sans);
  --t-body-l:  420 17px/1.55 var(--font-sans);
  --t-body:    420 15px/1.55 var(--font-sans);
  --t-small:   430 13px/1.45 var(--font-sans);
  --t-label:   600 11px/1   var(--font-sans);     /* uppercase tracked */

  /* ---- Spacing (4px base) ---- */
  --s-1: 4px;  --s-2: 8px;  --s-3: 12px; --s-4: 16px; --s-5: 20px;
  --s-6: 24px; --s-7: 32px; --s-8: 40px; --s-9: 48px; --s-10: 64px; --s-12: 80px;

  /* ---- Radius ---- */
  --r-xs: 5px; --r-sm: 8px; --r-md: 11px; --r-lg: 16px; --r-xl: 22px; --r-pill: 999px;

  /* ---- Elevation (light) ---- */
  --e-1: 0 1px 2px rgba(15,27,48,0.06), 0 1px 1px rgba(15,27,48,0.04);
  --e-2: 0 2px 4px rgba(15,27,48,0.06), 0 4px 12px rgba(15,27,48,0.07);
  --e-3: 0 6px 16px rgba(15,27,48,0.10), 0 12px 32px rgba(15,27,48,0.10);
  --e-4: 0 18px 48px rgba(10,27,61,0.18), 0 8px 20px rgba(10,27,61,0.10);

  /* ---- Motion ---- */
  --m-fast: 120ms;
  --m-base: 220ms;
  --m-slow: 360ms;
  --ease:        cubic-bezier(0.22, 0.8, 0.28, 1);
  --ease-out:    cubic-bezier(0.16, 1, 0.3, 1);
  --ease-spring: cubic-bezier(0.34, 1.56, 0.64, 1);
}

/* ============================================================
   DARK — FOCUS MODE
   ============================================================ */
[data-theme="focus"] {
  --bg:        #070E22;
  --bg-sunken: #050A1A;
  --surface:   #0E1A36;
  --surface-2: #122146;
  --surface-3: #16284C;
  --ink:       #EAF1FF;
  --ink-2:     #A8B8DA;
  --ink-3:     #7587AE;
  --ink-4:     #5A6B92;
  --border:    rgba(150,180,230,0.14);
  --border-2:  rgba(150,180,230,0.08);
  --primary:   #3B82F6;
  --primary-ink:#FFFFFF;
  --accent:    var(--cyan-400);
  --ring-focus: rgba(34,211,238,0.45);

  /* status backgrounds get dark, glassy fills */
  --st-pending-bg:  rgba(100,116,139,0.16);
  --st-progress:    #5B9BFF;
  --st-progress-bg: rgba(59,130,246,0.18);
  --st-done:        #2BD49A;
  --st-done-bg:     rgba(21,160,107,0.18);
  --st-human:       #FFB24D;
  --st-human-tint:  #FFC56E;
  --st-human-bg:    rgba(232,135,14,0.18);
  --st-human-glow:  rgba(255,178,77,0.55);
  --st-failed:      #FF6B6B;
  --st-failed-bg:   rgba(220,38,38,0.18);

  --rec-bid:#2BD49A; --rec-bid-bg:rgba(21,160,107,0.18);
  --rec-nobid:#FF7A8A; --rec-nobid-bg:rgba(194,53,74,0.18);
  --rec-review:#FFB24D; --rec-review-bg:rgba(232,135,14,0.18);

  --fit-strong:#2BD49A; --fit-good:#3DD6F0; --fit-fair:#FFB24D; --fit-weak:#FF7A8A;
  --fit-track: rgba(150,180,230,0.16);

  --urg-calm:#7587AE; --urg-soon:#5B9BFF; --urg-near:#FFB24D; --urg-crit:#FF6B6B;
  --urg-crit-bg: rgba(220,38,38,0.18);

  --e-1: 0 1px 2px rgba(0,0,0,0.4);
  --e-2: 0 4px 14px rgba(0,0,0,0.45);
  --e-3: 0 10px 30px rgba(0,0,0,0.5);
  --e-4: 0 24px 60px rgba(0,0,0,0.6);
}

/* ---- base reset ---- */
*, *::before, *::after { box-sizing: border-box; }
html { -webkit-text-size-adjust: 100%; }
body {
  margin: 0;
  font: var(--t-body);
  color: var(--ink);
  background: var(--bg);
  -webkit-font-smoothing: antialiased;
  text-rendering: optimizeLegibility;
}
h1,h2,h3,h4,p { margin: 0; }
button { font-family: inherit; }
::selection { background: var(--cyan-200); color: var(--blue-900); }
[data-theme="focus"] ::selection { background: rgba(34,211,238,0.3); color: #EAF1FF; }

.mono { font-family: var(--font-mono); font-feature-settings: "tnum" 1; }
.label {
  font: var(--t-label);
  text-transform: uppercase;
  letter-spacing: 0.09em;
  color: var(--ink-3);
}
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after { animation-duration: .001ms !important; transition-duration: .001ms !important; }
}
`

// componentStylesCSS is kaimi/ui.css from the handoff, verbatim: the shared
// component classes (status badge, recommendation pill, fit ring, deadline
// pill, buttons, avatar, chips, meta tags) built on the tokens above. Works in
// both themes.
const componentStylesCSS = `
/* ============================================================
   KAIMI — Shared UI components (works in both themes)
   Status vocabulary + core components. Single source of truth.
   ============================================================ */

/* ---------- STATUS BADGE (agent / stage state) ---------- */
.kbadge {
  display: inline-flex; align-items: center; gap: 6px;
  height: 24px; padding: 0 10px 0 8px;
  border-radius: var(--r-pill);
  font: 600 12px/1 var(--font-sans);
  letter-spacing: 0.01em; white-space: nowrap;
  border: 1px solid transparent;
}
.kbadge .dot {
  width: 7px; height: 7px; border-radius: 50%; background: currentColor;
  flex: none;
}
.kbadge--pending  { color: var(--st-pending);  background: var(--st-pending-bg);  border-color: color-mix(in oklab, var(--st-pending) 22%, transparent); }
.kbadge--progress { color: var(--st-progress); background: var(--st-progress-bg); border-color: color-mix(in oklab, var(--st-progress) 28%, transparent); }
.kbadge--done     { color: var(--st-done);     background: var(--st-done-bg);     border-color: color-mix(in oklab, var(--st-done) 28%, transparent); }
.kbadge--failed   { color: var(--st-failed);   background: var(--st-failed-bg);   border-color: color-mix(in oklab, var(--st-failed) 28%, transparent); }

/* Needs Human — the loudest. Solid amber, glow halo, gentle pulse. */
.kbadge--human {
  color: #fff;
  background: linear-gradient(180deg, var(--st-human-tint), var(--st-human));
  border-color: color-mix(in oklab, var(--st-human) 50%, transparent);
  box-shadow: 0 0 0 0 var(--st-human-glow);
  animation: kHumanPulse 2.2s var(--ease) infinite;
}
.kbadge--human .dot { background: #fff; box-shadow: 0 0 6px rgba(255,255,255,0.9); }
@keyframes kHumanPulse {
  0%   { box-shadow: 0 0 0 0 var(--st-human-glow); }
  60%  { box-shadow: 0 0 0 7px transparent; }
  100% { box-shadow: 0 0 0 0 transparent; }
}

/* In-progress dot animates */
.kbadge--progress .dot { animation: kBlink 1.3s ease-in-out infinite; }
@keyframes kBlink { 0%,100%{opacity:1} 50%{opacity:.35} }

/* ---------- RECOMMENDATION PILL ---------- */
.krec {
  display: inline-flex; align-items: center; gap: 6px;
  height: 26px; padding: 0 12px;
  border-radius: var(--r-sm);
  font: 700 12px/1 var(--font-sans);
  letter-spacing: 0.06em; text-transform: uppercase;
  border: 1px solid transparent;
}
.krec svg { width: 13px; height: 13px; }
.krec--bid    { color: var(--rec-bid);    background: var(--rec-bid-bg);    border-color: color-mix(in oklab, var(--rec-bid) 32%, transparent); }
.krec--nobid  { color: var(--rec-nobid);  background: var(--rec-nobid-bg);  border-color: color-mix(in oklab, var(--rec-nobid) 32%, transparent); }
.krec--review { color: var(--rec-review); background: var(--rec-review-bg); border-color: color-mix(in oklab, var(--rec-review) 36%, transparent); }

/* ---------- FIT-SCORE RING ---------- */
.kfit { position: relative; display: inline-grid; place-items: center; flex: none; }
.kfit svg { transform: rotate(-90deg); display: block; }
.kfit .kfit-track { stroke: var(--fit-track); }
.kfit .kfit-val { stroke-linecap: round; transition: stroke-dashoffset var(--m-slow) var(--ease); }
.kfit-num {
  position: absolute; display: grid; place-items: center; inset: 0;
  font-family: var(--font-mono); font-weight: 600; font-feature-settings: "tnum" 1;
  line-height: 1; color: var(--ink);
}
.kfit-num small { font-size: 0.5em; color: var(--ink-3); font-weight: 500; margin-top: 1px; }
.kfit[data-band="strong"] .kfit-val { stroke: var(--fit-strong); }
.kfit[data-band="good"]   .kfit-val { stroke: var(--fit-good); }
.kfit[data-band="fair"]   .kfit-val { stroke: var(--fit-fair); }
.kfit[data-band="weak"]   .kfit-val { stroke: var(--fit-weak); }

/* ---------- DEADLINE / URGENCY PILL ---------- */
.kdead {
  display: inline-flex; align-items: center; gap: 6px;
  height: 24px; padding: 0 10px;
  border-radius: var(--r-pill);
  font: 600 12px/1 var(--font-sans); white-space: nowrap;
  color: var(--urg-calm);
  background: color-mix(in oklab, var(--urg-calm) 12%, transparent);
}
.kdead svg { width: 13px; height: 13px; }
.kdead--soon { color: var(--urg-soon); background: color-mix(in oklab, var(--urg-soon) 12%, transparent); }
.kdead--near { color: var(--urg-near); background: color-mix(in oklab, var(--urg-near) 14%, transparent); }
.kdead--crit {
  color: #fff;
  background: linear-gradient(180deg, color-mix(in oklab, var(--urg-crit) 86%, white), var(--urg-crit));
  animation: kCritPulse 1.6s var(--ease) infinite;
}
@keyframes kCritPulse {
  0%,100% { box-shadow: 0 0 0 0 color-mix(in oklab, var(--urg-crit) 50%, transparent); }
  50%     { box-shadow: 0 0 0 5px transparent; }
}

/* ---------- BUTTONS ---------- */
.kbtn {
  --bh: 40px;
  display: inline-flex; align-items: center; justify-content: center; gap: 8px;
  height: var(--bh); padding: 0 18px;
  border-radius: var(--r-md); border: 1px solid transparent;
  font: 600 14px/1 var(--font-sans); cursor: pointer;
  transition: transform var(--m-fast) var(--ease), background var(--m-fast), box-shadow var(--m-fast), border-color var(--m-fast);
  white-space: nowrap; user-select: none;
}
.kbtn svg { width: 16px; height: 16px; }
.kbtn:active { transform: translateY(1px) scale(0.99); }
.kbtn:focus-visible { outline: none; box-shadow: 0 0 0 3px var(--ring-focus); }

.kbtn--ghost     { background: transparent; color: var(--ink-2); border-color: var(--border); }
.kbtn--ghost:hover { background: var(--surface-3); color: var(--ink); }
.kbtn--secondary { background: var(--surface); color: var(--ink); border-color: var(--border); box-shadow: var(--e-1); }
.kbtn--secondary:hover { border-color: var(--n-300); box-shadow: var(--e-2); }
.kbtn--primary   { background: var(--primary); color: var(--primary-ink); box-shadow: var(--e-2), inset 0 1px 0 rgba(255,255,255,0.18); }
.kbtn--primary:hover { background: color-mix(in oklab, var(--primary) 88%, black); box-shadow: var(--e-3); }

/* Select — the threshold-crossing action (cyan, confident) */
.kbtn--select {
  background: linear-gradient(180deg, var(--cyan-400), var(--cyan-500));
  color: #042530; font-weight: 700;
  box-shadow: 0 6px 18px rgba(14,165,196,0.4), inset 0 1px 0 rgba(255,255,255,0.5);
}
.kbtn--select:hover { box-shadow: 0 10px 26px rgba(14,165,196,0.5); transform: translateY(-1px); }

/* Approve — the gate's weighty green action */
.kbtn--approve {
  background: linear-gradient(180deg, color-mix(in oklab, var(--st-done) 88%, white), var(--st-done));
  color: #fff; font-weight: 700;
  box-shadow: 0 6px 18px color-mix(in oklab, var(--st-done) 45%, transparent), inset 0 1px 0 rgba(255,255,255,0.35);
}
.kbtn--approve:hover { transform: translateY(-1px); box-shadow: 0 10px 26px color-mix(in oklab, var(--st-done) 55%, transparent); }

/* Request changes — its weighty counterpart */
.kbtn--changes { background: var(--surface); color: var(--st-human); border-color: color-mix(in oklab, var(--st-human) 45%, transparent); }
.kbtn--changes:hover { background: var(--st-human-bg); }

.kbtn--lg { --bh: 52px; padding: 0 26px; font-size: 16px; border-radius: var(--r-lg); }
.kbtn--sm { --bh: 32px; padding: 0 12px; font-size: 13px; border-radius: var(--r-sm); }
.kbtn:disabled { opacity: 0.5; cursor: not-allowed; }

/* ---------- AVATAR (agent teammate) ---------- */
.kava {
  display: inline-grid; place-items: center; flex: none;
  width: 36px; height: 36px; border-radius: 11px;
  font: 700 13px/1 var(--font-sans); color: #fff;
  background: var(--blue-500); position: relative;
  box-shadow: inset 0 1px 0 rgba(255,255,255,0.2);
}
.kava--sm { width: 28px; height: 28px; border-radius: 9px; font-size: 11px; }
.kava--lg { width: 48px; height: 48px; border-radius: 14px; font-size: 17px; }

/* ---------- CHIP (filter / sort / meta) ---------- */
.kchip {
  display: inline-flex; align-items: center; gap: 6px;
  height: 30px; padding: 0 12px; border-radius: var(--r-pill);
  font: 500 13px/1 var(--font-sans); color: var(--ink-2);
  background: var(--surface); border: 1px solid var(--border);
  cursor: pointer; transition: all var(--m-fast) var(--ease);
}
.kchip:hover { border-color: var(--n-300); color: var(--ink); }
.kchip--on { background: var(--blue-50); color: var(--blue-600); border-color: color-mix(in oklab, var(--primary) 35%, transparent); }
[data-theme="focus"] .kchip--on { background: var(--st-progress-bg); color: #9CC2FF; }
.kchip svg { width: 14px; height: 14px; }

/* meta-row tag (NAICS etc) */
.ktag {
  display:inline-flex; align-items:center; gap:5px;
  font: 500 12px/1 var(--font-mono); color: var(--ink-3);
  padding: 3px 7px; border-radius: var(--r-xs);
  background: var(--surface-3); border: 1px solid var(--border-2);
}
`

// appStylesCSS is kaimi/app.css from the handoff, verbatim: the product
// app shell and screens built on the tokens — sidebar, page head, stat strip,
// toolbar (segmented filter + sort), opportunity row cards, proposal cards,
// workspace, review card, and drawer content blocks (GitHub issue #150).
const appStylesCSS = `
/* ============================================================
   KAIMI — App (minimal, light) · shell + screens
   Calmer than the hero: hairline borders, generous whitespace,
   color used only to carry status. Built on tokens.css / ui.css.
   ============================================================ */

:root{
  --app-bg: #FBFCFE;
  --line: rgba(16,30,60,0.08);
  --line-2: rgba(16,30,60,0.05);
  --ink-soft: #5A6B86;
  --sidebar: 248px;
}

body{ background: var(--app-bg); color: var(--ink); }

/* Fixed app shell: the grid is exactly the viewport height and never scrolls itself;
   the sidebar and the main column each scroll internally. This avoids the Firefox bug
   where a position:sticky grid item with height:100vh prevents the document from
   scrolling at all (Chrome tolerates it; Firefox/Zen do not). Mobile reverts to normal
   document scroll in the @media block below. */
.app{ display:grid; grid-template-columns: var(--sidebar) 1fr; height:100vh; overflow:hidden; }

/* ---------------- sidebar ---------------- */
.side{ border-right:1px solid var(--line); padding:22px 16px; display:flex; flex-direction:column; gap:6px;
  height:100vh; overflow-y:auto; background:#fff; }
.side .logo{ display:flex; align-items:center; gap:11px; padding:6px 8px 20px; }
.side .logo .mk{ width:34px;height:34px;border-radius:10px;flex:none;
  background:linear-gradient(150deg,var(--blue-600),var(--blue-900)); display:grid;place-items:center; }
.side .logo .nm{ font-weight:800; font-size:18px; letter-spacing:-0.02em; }
.side .logo .nm small{ display:block; font:500 11px/1 var(--font-sans); color:var(--ink-4); letter-spacing:0.04em; margin-top:3px; text-transform:uppercase; }

.nav-h{ font:600 11px/1 var(--font-sans); text-transform:uppercase; letter-spacing:0.1em; color:var(--ink-4); padding:14px 10px 8px; }
.nav-item{ display:flex; align-items:center; gap:12px; padding:10px 11px; border-radius:var(--r-md);
  color:var(--ink-2); font:550 14px/1 var(--font-sans); cursor:pointer; border:0; background:transparent; width:100%; text-align:left;
  transition: background var(--m-fast) var(--ease), color var(--m-fast); }
.nav-item svg{ width:19px; height:19px; color:var(--ink-4); flex:none; transition:color var(--m-fast); }
.nav-item:hover{ background:var(--n-50); color:var(--ink); }
.nav-item:hover svg{ color:var(--ink-3); }
.nav-item.on{ background:var(--blue-50); color:var(--blue-700); font-weight:600; }
.nav-item.on svg{ color:var(--blue-600); }
.nav-item .count{ margin-left:auto; font:600 12px/1 var(--font-mono); color:var(--ink-4); }
.nav-item .needs{ margin-left:auto; display:inline-flex; align-items:center; justify-content:center; min-width:20px; height:20px;
  padding:0 6px; border-radius:var(--r-pill); background:var(--st-human); color:#fff; font:700 11px/1 var(--font-sans); }
.side .spacer{ flex:1; }
.side .me{ display:flex; align-items:center; gap:11px; padding:10px; border-radius:var(--r-md); border:1px solid var(--line); }
.side .me .av{ width:32px;height:32px;border-radius:9px;flex:none; background:var(--blue-100); color:var(--blue-700);
  display:grid;place-items:center; font:700 13px/1 var(--font-sans); }
.side .me .who{ min-width:0; }
.side .me .who b{ font-size:13px; display:block; }
.side .me .who span{ font-size:11.5px; color:var(--ink-4); }

/* ---------------- main ---------------- */
.main{ min-width:0; height:100vh; overflow-y:auto; }
.page{ max-width:1080px; margin:0 auto; padding:38px 44px 80px; }
.page-head{ margin-bottom:30px; }
.page-head .eyebrow{ font:600 12px/1 var(--font-sans); letter-spacing:0.1em; text-transform:uppercase; color:var(--accent); }
.page-head h1{ font:700 30px/1.1 var(--font-sans); letter-spacing:-0.02em; margin-top:10px; }
.page-head .lead{ color:var(--ink-soft); font-size:15px; margin-top:8px; max-width:60ch; line-height:1.5; }

/* stat strip (minimal) */
.stats{ display:flex; gap:40px; margin-top:24px; flex-wrap:wrap; }
.stat .v{ font:700 30px/1 var(--font-sans); letter-spacing:-0.02em; }
.stat .v small{ font-size:15px; color:var(--ink-4); font-weight:600; margin-left:2px; white-space:nowrap; }
.stat .k{ font-size:13px; color:var(--ink-soft); margin-top:7px; }
.stat .v.amber{ color:var(--st-human); }

/* toolbar (filters/sort) */
.toolbar{ display:flex; align-items:center; gap:9px; margin:28px 0 14px; flex-wrap:wrap; }
.toolbar .grow{ flex:1; }
.seg{ display:inline-flex; gap:2px; padding:3px; border-radius:var(--r-pill); background:var(--n-100); border:1px solid var(--line); }
.seg button{ border:0; background:transparent; cursor:pointer; font:600 13px/1 var(--font-sans); color:var(--ink-3);
  padding:7px 13px; border-radius:var(--r-pill); transition:all var(--m-fast) var(--ease); white-space:nowrap; }
.seg button.on{ background:#fff; color:var(--ink); box-shadow:var(--e-1); }
.sortbtn{ display:inline-flex; align-items:center; gap:7px; height:34px; padding:0 12px; border-radius:var(--r-md);
  background:#fff; border:1px solid var(--line); color:var(--ink-2); font:550 13px/1 var(--font-sans); cursor:pointer; }
.sortbtn svg{ width:15px;height:15px; color:var(--ink-4); }
.sortbtn:hover{ border-color:var(--n-300); }

/* ---------------- opportunity list ---------------- */
.opp-list{ display:flex; flex-direction:column; }
.opp-list .day{ font:600 12px/1 var(--font-sans); text-transform:uppercase; letter-spacing:0.08em; color:var(--ink-4);
  padding:22px 4px 12px; display:flex; align-items:center; gap:8px; }
.opp-list .day .ln{ flex:1; height:1px; background:var(--line-2); }

.orow{ display:flex; align-items:center; gap:20px; padding:18px 18px; border-radius:var(--r-lg);
  background:#fff; border:1px solid var(--line); cursor:pointer; margin-bottom:10px;
  transition: border-color var(--m-base) var(--ease), box-shadow var(--m-base), transform var(--m-base); }
.orow:hover{ border-color:var(--blue-200); box-shadow:var(--e-2); }
.orow:focus-visible, .pcard:focus-visible, .nav-item:focus-visible, .seg button:focus-visible{
  outline:none; box-shadow:0 0 0 3px var(--ring-focus); border-color:var(--blue-300); }
.orow .newdot{ width:7px;height:7px;border-radius:50%;background:var(--cyan-400); flex:none; visibility:hidden; }
.orow.new .newdot{ visibility:visible; box-shadow:0 0 0 3px var(--cyan-50); }
.orow .ttl{ font:600 16px/1.3 var(--font-sans); letter-spacing:-0.01em; min-width:0; }
.orow .body{ flex:1; min-width:0; }
.orow .meta{ display:flex; align-items:center; gap:9px; margin-top:6px; color:var(--ink-soft); font-size:13px; }
.orow .meta .sep{ width:3px;height:3px;border-radius:50%;background:var(--ink-4); flex:none; }
.orow .meta .naics{ font-family:var(--font-mono); font-size:12px; color:var(--ink-4); }
.orow .right{ display:flex; align-items:center; gap:18px; flex:none; }
.orow .chev{ width:18px;height:18px;color:var(--ink-4); flex:none; }
.orow .rec-min{ font:700 12px/1 var(--font-sans); text-transform:uppercase; letter-spacing:0.05em; white-space:nowrap; }
.rec-min--bid{ color:var(--rec-bid); } .rec-min--nobid{ color:var(--rec-nobid); } .rec-min--review{ color:var(--rec-review); }

/* ---------------- search box (toolbar) ---------------- */
.searchbox{ display:flex; align-items:center; gap:9px; height:38px; padding:0 13px; border-radius:var(--r-md);
  border:1px solid var(--line); background:#fff; min-width:300px;
  transition:border-color var(--m-fast), box-shadow var(--m-fast); }
.searchbox:focus-within{ border-color:var(--blue-300); box-shadow:0 0 0 3px var(--ring-focus); }
.searchbox svg{ width:15px; height:15px; color:var(--ink-4); flex:none; }
.searchbox input{ border:0; outline:none; background:transparent; font:500 13.5px/1 var(--font-sans); color:var(--ink); width:100%; }
.searchbox input::placeholder{ color:var(--ink-4); }

/* ---------------- submitted archive rows ---------------- */
.sub-list{ display:flex; flex-direction:column; gap:10px; }
.srow{ background:#fff; border:1px solid var(--line); border-radius:var(--r-lg); overflow:hidden;
  transition:border-color var(--m-base) var(--ease), box-shadow var(--m-base); }
.srow:hover{ border-color:var(--blue-200); }
.srow.open{ border-color:var(--blue-200); box-shadow:var(--e-2); }
.srow.justnow{ border-color:color-mix(in oklab,var(--st-done) 38%,transparent); }
.srow-head{ display:flex; align-items:center; gap:18px; padding:16px 20px; cursor:pointer; }
.srow-head:focus-visible{ outline:none; box-shadow:inset 0 0 0 3px var(--ring-focus); }
.srow .s-body{ flex:1; min-width:0; }
.srow .sttl{ font:600 15.5px/1.3 var(--font-sans); letter-spacing:-0.01em; }
.srow .smeta{ display:flex; align-items:center; gap:9px; margin-top:5px; color:var(--ink-soft); font-size:12.5px; flex-wrap:wrap; }
.srow .smeta .sep{ width:3px;height:3px;border-radius:50%;background:var(--ink-4); flex:none; }
.srow .smeta .mono{ font-family:var(--font-mono); font-size:11.5px; color:var(--ink-4); }
.srow .s-right{ display:flex; align-items:center; gap:16px; flex:none; }
.srow .sval{ font:600 14px/1 var(--font-mono); color:var(--ink); min-width:52px; text-align:right; }
.srow .schev{ width:18px; height:18px; color:var(--ink-4); flex:none; transition:transform var(--m-base) var(--ease); }
.srow .schev.up{ transform:rotate(180deg); }
.srow-docs{ padding:14px 20px 18px; border-top:1px dashed var(--line); animation:routeIn .24s var(--ease) both; }
.srow-docs .sd-note{ font:500 12.5px/1 var(--font-sans); color:var(--ink-soft); }
.srow-docs .sd-h{ font:600 11px/1 var(--font-sans); text-transform:uppercase; letter-spacing:0.09em; color:var(--ink-4); margin-top:14px; }
.kbadge--muted{ color:var(--ink-4); background:var(--n-100); border-color:var(--n-200); }
.srow-docs .sd-grid{ display:grid; grid-template-columns:1fr 280px; gap:24px; margin-top:14px; align-items:start; }
.srow-docs .sd-grid .sd-h{ margin-top:0; }
.sd-outcome .sd-hint{ font:450 11.5px/1.5 var(--font-sans); color:var(--ink-4); margin-top:10px; }
@media (max-width:980px){ .srow-docs .sd-grid{ grid-template-columns:1fr; } }

/* ---------------- export report dialog ---------------- */
.xscrim{ position:fixed; inset:0; z-index:60; background:rgba(7,16,38,0.42); display:grid; place-items:center;
  animation:xFade .2s var(--ease) both; }
@keyframes xFade{ from{opacity:0;} to{opacity:1;} }
.xmodal{ width:480px; max-width:calc(100vw - 48px); background:#fff; border-radius:var(--r-xl, 20px);
  box-shadow:var(--e-4, 0 24px 64px rgba(7,16,38,0.28)); padding:28px 28px 24px; animation:routeIn .26s var(--ease) both; }
.xmodal h3{ font:700 19px/1.2 var(--font-sans); letter-spacing:-0.01em; margin:0; }
.xmodal > p{ font:450 13.5px/1.55 var(--font-sans); color:var(--ink-soft); margin:8px 0 0; }
.x-periods{ display:grid; grid-template-columns:repeat(auto-fit, minmax(120px,1fr)); gap:8px; margin-top:18px; }
.x-period{ border:1px solid var(--line); background:#fff; border-radius:var(--r-md); padding:10px 12px; cursor:pointer;
  font:600 12.5px/1.2 var(--font-sans); color:var(--ink-2); text-align:left; transition:all var(--m-fast); }
.x-period:hover{ border-color:var(--blue-200); }
.x-period.on{ border-color:var(--blue-400, var(--blue-300)); background:var(--blue-50); color:var(--blue-700); box-shadow:0 0 0 1px var(--blue-300) inset; }
.x-qh{ font:550 11.5px/1.4 var(--font-sans); color:var(--ink-4); margin-top:16px; }
.x-qh b{ color:var(--blue-700); font-weight:700; }
.x-qstrip{ display:flex; gap:6px; flex-wrap:wrap; margin-top:8px; }
.x-q{ border:1px solid var(--line); background:#fff; border-radius:var(--r-pill); padding:7px 12px; cursor:pointer;
  font:600 12px/1 var(--font-mono); color:var(--ink-3); transition:all var(--m-fast); }
.x-q:hover{ border-color:var(--blue-200); color:var(--ink); }
.x-q.on{ background:var(--blue-50); border-color:var(--blue-300); color:var(--blue-700); }
.x-q.anchor{ box-shadow:0 0 0 2px var(--blue-300); }
.x-metrics{ display:grid; grid-template-columns:repeat(4,1fr); gap:8px; margin-top:14px;
  background:var(--n-50); border:1px solid var(--line); border-radius:var(--r-lg); padding:14px 16px; }
.x-metrics .xm b{ display:block; font:700 19px/1 var(--font-sans); letter-spacing:-0.01em; }
.x-metrics .xm span{ display:block; font:550 10.5px/1 var(--font-sans); text-transform:uppercase; letter-spacing:0.07em; color:var(--ink-4); margin-top:6px; }
.x-actions{ display:flex; justify-content:flex-end; gap:10px; margin-top:20px; }


/* ---------------- proposal cards (active) ---------------- */
.prop-grid{ display:flex; flex-direction:column; gap:12px; }
.pcard{ display:flex; align-items:center; gap:22px; padding:20px 22px; border-radius:var(--r-lg);
  background:#fff; border:1px solid var(--line); cursor:pointer;
  transition: border-color var(--m-base) var(--ease), box-shadow var(--m-base); }
.pcard:hover{ border-color:var(--blue-200); box-shadow:var(--e-2); }
.pcard.needs{ border-color:color-mix(in oklab,var(--st-human) 40%,transparent); background:linear-gradient(90deg,var(--st-human-bg),#fff 38%); }
.pcard .pc-body{ flex:1; min-width:0; }
.pcard .pc-ttl{ font:600 16px/1.3 var(--font-sans); letter-spacing:-0.01em; }
.pcard .pc-agency{ font-size:13px; color:var(--ink-soft); margin-top:5px; }
.pcard .pc-right{ display:flex; align-items:center; gap:20px; flex:none; }
.pcard .pc-when{ font-size:12px; color:var(--ink-4); white-space:nowrap; }
.pcard .pc-pipe{ display:flex; flex-direction:column; gap:0; align-items:flex-start; width:186px; flex:none; }
.pcard .pc-working{ display:inline-flex; align-items:center; gap:7px; font:600 12.5px/1 var(--font-sans); color:var(--st-progress); white-space:nowrap; }
.pcard .pc-working .pulse{ width:7px;height:7px;border-radius:50%;background:var(--st-progress); animation:kBlink 1.3s infinite; }
.section-h{ display:flex; align-items:center; gap:9px; margin:28px 0 14px; }
.section-h .lbl{ font:600 12px/1 var(--font-sans); text-transform:uppercase; letter-spacing:0.08em; color:var(--ink-soft); }
.section-h .lbl.amber{ color:var(--st-human); }
.section-h .cnt{ font:600 12px/1 var(--font-mono); color:var(--ink-4); }
.section-h .ln{ flex:1; height:1px; background:var(--line-2); }

/* mini pipeline (dots) */
.minipipe{ display:flex; align-items:center; gap:0; }
.minipipe .seg{ width:30px; height:3px; border-radius:2px; background:var(--n-200); padding:0; border:0; }
.minipipe .seg.done{ background:var(--st-done); }
.minipipe .seg.active{ background:var(--st-progress); }
.minipipe .seg.human{ background:var(--st-human); }
.minipipe .node{ width:9px;height:9px;border-radius:50%;background:var(--n-300); flex:none; z-index:1; }
.minipipe .node.done{ background:var(--st-done); }
.minipipe .node.active{ background:var(--st-progress); box-shadow:0 0 0 3px var(--st-progress-bg); }
.minipipe .node.human{ background:var(--st-human); box-shadow:0 0 0 3px var(--st-human-bg); }
.stage-label{ font:600 12.5px/1 var(--font-sans); color:var(--ink-2); margin-top:9px; }
.stage-label.human{ color:var(--st-human); }

.needs-tag{ display:inline-flex; align-items:center; gap:6px; height:26px; padding:0 11px 0 9px; border-radius:var(--r-pill);
  background:var(--st-human); color:#fff; font:700 12px/1 var(--font-sans); white-space:nowrap; }
.needs-tag svg{ width:13px;height:13px; }

/* ---------------- workspace ---------------- */
.ws{ max-width:920px; margin:0 auto; padding:34px 44px 90px; }
.ws .back{ display:inline-flex; align-items:center; gap:7px; color:var(--ink-soft); font:550 13px/1 var(--font-sans);
  background:transparent; border:0; cursor:pointer; padding:6px 8px 6px 0; white-space:nowrap; }
.ws .back svg{ width:16px;height:16px; }
.ws .back:hover{ color:var(--ink); }
.ws-head{ display:flex; align-items:flex-start; gap:22px; margin:14px 0 30px; }
.ws-head .ws-id{ min-width:0; flex:1; }
.ws-head h1{ font:700 27px/1.15 var(--font-sans); letter-spacing:-0.02em; max-width:26ch; }
.ws-head .ws-meta{ display:flex; align-items:center; gap:9px; margin-top:11px; color:var(--ink-soft); font-size:13.5px; flex-wrap:wrap; }
.ws-head .ws-meta .sep{ width:3px;height:3px;border-radius:50%;background:var(--ink-4); }

/* big horizontal pipeline (calm, light) */
.wpipe{ display:flex; align-items:flex-start; gap:0; padding:6px 4px 4px; margin-bottom:6px; }
.wnode{ display:flex; flex-direction:column; align-items:center; gap:9px; width:120px; flex:none; }
.wring{ width:44px;height:44px;border-radius:50%; display:grid;place-items:center; background:#fff;
  border:1.5px solid var(--n-200); color:var(--ink-4); transition: all var(--m-base) var(--ease); }
.wring svg{ width:18px;height:18px; }
.wnode .wname{ font:600 12.5px/1.2 var(--font-sans); color:var(--ink-3); text-align:center; white-space:nowrap; }
.wnode .wstate{ font:600 10px/1 var(--font-sans); text-transform:uppercase; letter-spacing:0.05em; color:var(--ink-4); }
.wnode[data-st="done"] .wring{ border-color:var(--st-done); background:var(--st-done-bg); color:var(--st-done); }
.wnode[data-st="done"] .wname{ color:var(--ink-2); } .wnode[data-st="done"] .wstate{ color:var(--st-done); }
.wnode[data-st="progress"] .wring{ border-color:var(--st-progress); background:var(--st-progress-bg); color:var(--st-progress); box-shadow:0 0 0 4px var(--st-progress-bg); }
.wnode[data-st="progress"] .wname{ color:var(--ink); } .wnode[data-st="progress"] .wstate{ color:var(--st-progress); }
.wnode[data-st="progress"] .wring svg{ animation:spin 2.4s linear infinite; }
.wnode[data-st="human"] .wring{ border-color:transparent; background:var(--st-human); color:#fff; box-shadow:0 6px 16px var(--st-human-glow); transform:scale(1.05); }
.wnode[data-st="human"] .wname{ color:var(--st-human); font-weight:700; } .wnode[data-st="human"] .wstate{ color:var(--st-human); }
.wconn{ flex:1; min-width:14px; height:2px; margin-top:21px; border-radius:2px; background:var(--n-200); }
.wconn[data-on="done"]{ background:var(--st-done); }
@keyframes spin{ to{ transform:rotate(360deg);} }

/* review card (clean light handoff) */
.review{ border-radius:var(--r-xl); border:1px solid color-mix(in oklab,var(--st-human) 35%,transparent); background:#fff;
  box-shadow:var(--e-2); overflow:hidden; margin-top:26px; }
@media (prefers-reduced-motion: no-preference){ .review{ animation: rUp .45s var(--ease-spring) both; } }
@keyframes rUp{ from{ transform:translateY(12px);} to{ transform:none;} }
.review .r-head{ display:flex; align-items:center; gap:15px; padding:22px 26px; background:linear-gradient(180deg,var(--st-human-bg),#fff); border-bottom:1px solid var(--line); }
.review .r-badge{ display:inline-flex; align-items:center; gap:7px; padding:7px 12px 7px 10px; border-radius:var(--r-pill);
  background:var(--st-human); color:#fff; font:700 12px/1 var(--font-sans); text-transform:uppercase; letter-spacing:0.05em; flex:none; }
.review .r-badge svg{ width:14px;height:14px; }
.review .r-head h2{ font:700 19px/1.2 var(--font-sans); letter-spacing:-0.01em; }
.review .r-head p{ font-size:13.5px; color:var(--ink-soft); margin-top:5px; }
.review .r-hand{ margin-left:auto; display:flex; align-items:center; gap:11px; flex:none; }
.review .r-hand .av{ width:42px;height:42px;border-radius:12px; display:grid;place-items:center; color:#fff; font:700 15px/1 var(--font-sans); }
.review .r-hand .arrow{ color:var(--st-human); }
.review .r-hand .you{ width:42px;height:42px;border-radius:12px; display:grid;place-items:center; background:var(--st-human); color:#fff; }
.review .r-hand .you svg{ width:21px;height:21px; }

.review .r-body{ padding:24px 26px; }
.review .r-sec-h{ font:600 11px/1 var(--font-sans); text-transform:uppercase; letter-spacing:0.09em; color:var(--ink-4); margin-bottom:13px; }
.review .summary{ font:420 15.5px/1.6 var(--font-sans); color:var(--ink); max-width:70ch; }
.review .crit2{ display:grid; grid-template-columns:1fr 1fr; gap:9px; margin-top:8px; }
@media (max-width:720px){ .review .crit2{ grid-template-columns:1fr; } }
.citem{ display:flex; gap:11px; align-items:flex-start; padding:12px 14px; border-radius:var(--r-md); background:var(--n-25); border:1px solid var(--line); }
.citem .ci-ic{ width:22px;height:22px;border-radius:7px;flex:none;display:grid;place-items:center; }
.citem.ok .ci-ic{ background:var(--st-done-bg); color:var(--st-done); } .citem.warn .ci-ic{ background:var(--st-human-bg); color:var(--st-human); }
.citem .ci-ic svg{ width:13px;height:13px; }
.citem .ci-l{ font:600 13.5px/1.3 var(--font-sans); } .citem .ci-n{ font-size:12px; color:var(--ink-soft); margin-top:3px; line-height:1.4; }

.gapflag{ display:flex; gap:13px; margin-top:18px; padding:15px 17px; border-radius:var(--r-md);
  background:var(--st-human-bg); border:1px solid color-mix(in oklab,var(--st-human) 30%,transparent); }
.gapflag .gf-ic{ width:32px;height:32px;border-radius:9px;flex:none;display:grid;place-items:center; background:color-mix(in oklab,var(--st-human) 20%,transparent); color:var(--st-human); }
.gapflag .gf-t{ font:650 14px/1.3 var(--font-sans); color:color-mix(in oklab,var(--st-human) 75%,black); }
.gapflag .gf-d{ font-size:13px; color:var(--ink-soft); margin-top:4px; line-height:1.5; }

.artifact2{ display:inline-flex; align-items:center; gap:9px; padding:9px 13px; border-radius:var(--r-md);
  border:1px solid var(--line); background:#fff; font-size:13px; color:var(--ink-2); cursor:pointer; text-decoration:none;
  transition: all var(--m-fast); }
.artifact2:hover{ border-color:var(--blue-200); color:var(--ink); }
.artifact2 svg{ width:15px;height:15px;color:var(--blue-500); }
.art-row{ display:flex; gap:10px; flex-wrap:wrap; margin-top:10px; }

.review .r-actions{ display:flex; align-items:center; gap:12px; padding:18px 26px 22px; border-top:1px solid var(--line); background:var(--n-25); }
.review .r-actions .note{ margin-left:auto; font-size:12.5px; color:var(--ink-4); max-width:30ch; text-align:right; line-height:1.4; }

/* calm working / done states in workspace */
.ws-state{ border-radius:var(--r-xl); border:1px solid var(--line); background:#fff; box-shadow:var(--e-1);
  padding:30px 28px; margin-top:26px; display:flex; gap:18px; align-items:flex-start; }
.ws-state .ws-av{ width:48px;height:48px;border-radius:14px; display:grid;place-items:center; color:#fff; font:700 17px/1 var(--font-sans); flex:none; }
.ws-state h3{ font:650 18px/1.2 var(--font-sans); }
.ws-state .role{ font-size:13px; color:var(--ink-soft); margin-top:4px; }
.ws-state .desc{ font-size:14.5px; color:var(--ink-2); line-height:1.55; margin-top:14px; max-width:64ch; }

/* empty state */
.empty2{ text-align:center; padding:64px 20px; display:flex; flex-direction:column; align-items:center; gap:13px; }
.empty2 .g{ width:60px;height:60px;border-radius:17px; background:var(--n-100); display:grid;place-items:center; color:var(--ink-4); }
.empty2 h3{ font:650 18px/1.2 var(--font-sans); } .empty2 p{ color:var(--ink-soft); font-size:14px; max-width:38ch; }

@media (max-width:860px){
  /* On narrow screens revert to normal document scroll: the sidebar becomes a top bar
     and the page (body) scrolls, so the fixed-shell internal-scroll does not apply. */
  .app{ grid-template-columns:1fr; height:auto; overflow:visible; }
  .side{ position:static; height:auto; overflow:visible; flex-direction:row; align-items:center; flex-wrap:wrap; }
  .main{ height:auto; overflow:visible; }
  .side .spacer, .side .me, .side .nav-h{ display:none; }
}

/* ---------------- opportunity drawer ---------------- */
.drawer-scrim{ position:fixed; inset:0; z-index:60; background:rgba(10,27,61,0.32); backdrop-filter:blur(2px);
  display:flex; justify-content:flex-end; }
.drawer{ width:min(486px,100%); height:100%; background:#fff; display:flex; flex-direction:column;
  box-shadow:var(--e-4); }
@media (prefers-reduced-motion: no-preference){ .drawer{ animation: drawerIn var(--m-slow) var(--ease-out) both; } }
@keyframes drawerIn{ from{ transform:translateX(28px); } to{ transform:none; } }
.dr-head{ display:flex; align-items:center; gap:12px; padding:18px 24px; border-bottom:1px solid var(--line); }
.dr-close{ width:34px;height:34px;border-radius:var(--r-md); border:1px solid var(--line); background:#fff; cursor:pointer;
  display:grid;place-items:center; color:var(--ink-3); }
.dr-close svg{ width:16px;height:16px; } .dr-close:hover{ background:var(--n-50); }
.dr-head .rec-min{ margin-left:6px; }
.dr-head .kdead{ margin-left:auto; }
.dr-body{ flex:1; overflow-y:auto; padding:26px 28px; }
.dr-top{ display:flex; gap:20px; align-items:center; }
.dr-top h2{ font:700 21px/1.2 var(--font-sans); letter-spacing:-0.02em; max-width:22ch; }
.dr-sub{ font-size:14px; color:var(--ink-soft); margin-top:7px; }
.dr-tags{ display:flex; gap:7px; margin-top:11px; flex-wrap:wrap; }
.dr-sec-h{ font:600 11px/1 var(--font-sans); text-transform:uppercase; letter-spacing:0.09em; color:var(--ink-4); margin:28px 0 13px; }
.reasons{ list-style:none; padding:0; margin:0; display:flex; flex-direction:column; gap:11px; }
.reasons li{ display:flex; gap:11px; font-size:14px; color:var(--ink-2); line-height:1.5; }
.reasons li .rd{ width:6px;height:6px;border-radius:50%;background:var(--accent); margin-top:7px; flex:none; }
.musts{ display:flex; flex-direction:column; gap:8px; }
.must{ display:flex; align-items:center; gap:11px; padding:11px 13px; border-radius:var(--r-md); border:1px solid var(--line);
  background:var(--n-25); font-size:13.5px; font-weight:500; }
.must .mc{ width:20px;height:20px;border-radius:6px;flex:none;display:grid;place-items:center; }
.must.ok .mc{ background:var(--st-done-bg); color:var(--st-done); } .must.no .mc{ background:var(--st-human-bg); color:var(--st-human); }
.must .mc svg{ width:12px;height:12px; }
.dr-actions{ display:flex; align-items:center; gap:12px; padding:18px 24px; border-top:1px solid var(--line); }
`

// editorStylesCSS is kaimi/editor.css from the updated handoff: the full-page
// draft editor shell (.ed-fullpage, .ed-rail section list, .ed-doc editable
// sections, .ed-flag gap callouts). Emitted by StyleTag so the editor route is
// styled like every other surface.
const editorStylesCSS = `
/* app-shell route motion — from Kaimi App.html root <style>; ported here so
   StyleTag carries it on every page (shell + the non-shell editor route).
   .srow-docs and .xmodal reference @keyframes routeIn. */
#root{ min-height:100vh; }
.route-fade{ animation: routeIn .28s var(--ease) both; }
@keyframes routeIn{ from{ transform:translateY(6px); } to{ transform:none; } }

/* ============================================================
   KAIMI — Working-draft editor styles (web)
   Mirror of the editor section in desktop.css — keep in sync.
   ============================================================ */
.ed-fullpage{ height:100vh; }

/* ============================================================
   DRAFT EDITOR
   ============================================================ */
.ed{ display:grid; grid-template-columns:252px 1fr; height:100%; min-height:0; }
.ed-rail{ border-right:1px solid var(--line, rgba(16,30,60,0.08)); background:#fff; overflow-y:auto; padding:14px 12px; }
.ed-rail .er-h{ font:600 11px/1 var(--font-sans); text-transform:uppercase; letter-spacing:0.09em; color:var(--ink-4); padding:8px 10px 10px; }
.ed-sec{ display:flex; align-items:flex-start; gap:10px; width:100%; text-align:left; padding:9px 10px; border-radius:var(--r-md);
  border:0; background:transparent; cursor:pointer; transition:background var(--m-fast); }
.ed-sec:hover{ background:var(--n-50); }
.ed-sec.cur{ background:var(--blue-50); }
.ed-sec .dot{ width:8px; height:8px; border-radius:50%; margin-top:5px; flex:none; background:var(--st-done); }
.ed-sec.warn .dot{ background:var(--st-human); box-shadow:0 0 0 3px var(--st-human-bg); }
.ed-sec b{ font:600 13px/1.35 var(--font-sans); color:var(--ink-2); display:block; }
.ed-sec.cur b{ color:var(--blue-700); }
.ed-sec .sub{ font:500 11px/1 var(--font-mono); color:var(--ink-4); display:block; margin-top:4px; }

.ed-main{ display:flex; flex-direction:column; min-width:0; min-height:0; }
.ed-top{ flex:none; display:flex; align-items:center; gap:14px; padding:12px 22px; border-bottom:1px solid var(--line, rgba(16,30,60,0.08)); background:#fff; }
.ed-top .ed-name{ font:650 14.5px/1.2 var(--font-sans); }
.ed-top .ed-name span{ display:block; font:500 11.5px/1 var(--font-mono); color:var(--ink-4); margin-top:4px; }
.ed-ver{ display:inline-flex; align-items:center; gap:6px; height:26px; padding:0 11px; border-radius:var(--r-pill);
  background:var(--n-100); font:600 12px/1 var(--font-sans); color:var(--ink-2); white-space:nowrap; }
.ed-ver.you{ background:var(--blue-50); color:var(--blue-700); }
.ed-save{ font:550 12px/1 var(--font-sans); color:var(--ink-4); display:inline-flex; align-items:center; gap:6px; white-space:nowrap; }
.ed-save .sdot{ width:7px; height:7px; border-radius:50%; background:var(--st-done); }
.ed-save.saving .sdot{ background:var(--st-progress); animation:kBlink 0.9s infinite; }
.ed-scroll{ flex:1; overflow-y:auto; padding:30px 22px 80px; background:var(--app-bg, #FBFCFE); }
.ed-doc{ max-width:740px; margin:0 auto; background:#fff; border:1px solid var(--line, rgba(16,30,60,0.08));
  border-radius:var(--r-lg); box-shadow:var(--e-2); padding:52px 58px 64px; }
.ed-doc .ed-title{ font:750 23px/1.25 var(--font-sans); letter-spacing:-0.015em; }
.ed-doc .ed-meta{ font:500 11.5px/1.6 var(--font-mono); color:var(--ink-4); margin-top:8px; padding-bottom:22px; border-bottom:1px solid var(--line, rgba(16,30,60,0.08)); }
.ed-doc section{ margin-top:30px; }
.ed-doc .sec-head2{ display:flex; align-items:baseline; gap:10px; }
.ed-doc h3{ font:700 16px/1.3 var(--font-sans); color:var(--ink); }
.ed-doc h3 .no{ color:var(--cyan-600); font-family:var(--font-mono); font-weight:600; font-size:13px; margin-right:8px; }
.ed-doc .reqtag{ margin-left:auto; font:500 10.5px/1 var(--font-mono); color:var(--ink-4); background:var(--n-50);
  border:1px solid var(--line, rgba(16,30,60,0.08)); padding:4px 8px; border-radius:var(--r-pill); white-space:nowrap; flex:none; }
.ed-doc [contenteditable]{ font:450 14.5px/1.75 var(--font-sans); color:var(--ink-2); margin-top:10px; outline:none;
  border-radius:6px; transition:box-shadow var(--m-fast); }
.ed-doc [contenteditable] p{ margin:0 0 12px; }
.ed-doc [contenteditable]:hover{ box-shadow:0 0 0 6px var(--n-25), 0 0 0 7px var(--n-200); }
.ed-doc [contenteditable]:focus{ box-shadow:0 0 0 6px var(--n-25), 0 0 0 7px var(--cyan-300); color:var(--ink); }
.ed-flag{ display:flex; gap:12px; margin-top:14px; padding:13px 15px; border-radius:var(--r-md);
  background:var(--st-human-bg); border:1px solid color-mix(in oklab,var(--st-human) 32%,transparent); }
.ed-flag .ef-ic{ width:30px; height:30px; border-radius:9px; flex:none; display:grid; place-items:center;
  background:color-mix(in oklab,var(--st-human) 20%,transparent); color:var(--st-human); }
.ed-flag b{ font:650 13px/1.35 var(--font-sans); color:color-mix(in oklab,var(--st-human) 75%,black); display:block; }
.ed-flag p{ font:450 12.5px/1.5 var(--font-sans); color:var(--ink-soft); margin:3px 0 0; }

/* Unresolved Writer gaps (issue #269): amber-tint the section editor, give each
   gap a callout with a jump control, and mark gaps inline in read-only views. */
.edsec textarea.gap-warn{ border-color:color-mix(in oklab,var(--st-human) 55%,transparent);
  background:color-mix(in oklab,var(--st-human-bg) 55%,var(--surface)); }
.edsec textarea.gap-warn:focus{ border-color:var(--st-human); }
.ed-gap{ align-items:center; }
.ed-gap > div{ flex:1; }
.ed-gap .gap-jump{ flex:none; color:color-mix(in oklab,var(--st-human) 75%,black); }
.gap-mark{ background:var(--st-human-bg); color:color-mix(in oklab,var(--st-human) 75%,black);
  border:1px solid color-mix(in oklab,var(--st-human) 35%,transparent); border-radius:4px; padding:0 4px; font-weight:600; }
`

// StyleTag renders the complete Kaimi design system — tokens plus component
// classes — as a single inline <style> element for a page's <head>. This is
// the one place visual values are defined; see the file comment for the rules.
func StyleTag() template.HTML {
	// #nosec G203 -- constant stylesheets plus base64-embedded fonts, no user input.
	// fontFaceCSS is emitted first so the @font-face declarations exist before any
	// rule references --font-sans / --font-mono.
	return template.HTML("<style>" + fontFaceCSS + designTokensCSS + componentStylesCSS + appStylesCSS + editorStylesCSS + "</style>")
}
