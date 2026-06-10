/* ============================================================
   KAIMI — shared design-system React components.
   Ported verbatim from the design handoff (lifecycle-components.jsx);
   the agent state machine (useLifecycle) is omitted — the desktop app
   drives the pipeline from proposal status directly.
   ============================================================ */
import React from 'react';

/* ---------- icons ---------- */
export const I = {
  check: (p) => <svg viewBox="0 0 24 24" fill="none" {...p}><path d="M5 13l4 4L19 7" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round"/></svg>,
  spinner: (p) => <svg viewBox="0 0 24 24" fill="none" {...p}><path d="M12 3v3M12 18v3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M3 12h3M18 12h3M5.6 18.4l2.1-2.1M16.3 7.7l2.1-2.1" stroke="currentColor" strokeWidth="2.1" strokeLinecap="round"/></svg>,
  dot: (p) => <svg viewBox="0 0 24 24" {...p}><circle cx="12" cy="12" r="4" fill="currentColor"/></svg>,
  star: (p) => <svg viewBox="0 0 24 24" fill="none" {...p}><path d="M12 2.5l2.6 5.5 6 .8-4.4 4.2 1.1 6L12 16.9 6.7 19l1.1-6L3.4 8.8l6-.8z" fill="currentColor"/></svg>,
  arrow: (p) => <svg viewBox="0 0 24 24" fill="none" {...p}><path d="M5 12h14M13 6l6 6-6 6" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round"/></svg>,
  clock: (p) => <svg viewBox="0 0 24 24" fill="none" {...p}><circle cx="12" cy="13" r="8" stroke="currentColor" strokeWidth="2"/><path d="M12 9v4l3 2" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/></svg>,
  flag: (p) => <svg viewBox="0 0 24 24" fill="none" {...p}><path d="M5 21V4M5 5h11l-1.5 3.5L16 12H5" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/></svg>,
  doc: (p) => <svg viewBox="0 0 24 24" fill="none" {...p}><path d="M7 3h7l5 5v13H7z" stroke="currentColor" strokeWidth="1.8" strokeLinejoin="round"/><path d="M14 3v5h5" stroke="currentColor" strokeWidth="1.8" strokeLinejoin="round"/></svg>,
  warn: (p) => <svg viewBox="0 0 24 24" fill="none" {...p}><path d="M12 4l9 16H3z" stroke="currentColor" strokeWidth="1.9" strokeLinejoin="round"/><path d="M12 10v4M12 17v.01" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/></svg>,
  link: (p) => <svg viewBox="0 0 24 24" fill="none" {...p}><path d="M10 14a4 4 0 0 0 5.7 0l2.3-2.3a4 4 0 1 0-5.7-5.7L11 7.3M14 10a4 4 0 0 0-5.7 0L6 12.3a4 4 0 1 0 5.7 5.7l1.3-1.3" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round"/></svg>,
  back: (p) => <svg viewBox="0 0 24 24" fill="none" {...p}><path d="M19 12H5M11 18l-6-6 6-6" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round"/></svg>,
  hand: (p) => <svg viewBox="0 0 24 24" fill="none" {...p}><path d="M8 11V5.5a1.5 1.5 0 0 1 3 0V10m0-1V4.5a1.5 1.5 0 0 1 3 0V10m0-.5V6a1.5 1.5 0 0 1 3 0v7c0 3.5-2.2 7-6.5 7C10 20 8.4 18.6 7 16.5l-2.2-3.4a1.5 1.5 0 0 1 2.4-1.8L8 12" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"/></svg>,
};

/* ---------- fit band ---------- */
export function fitBand(v){ return v>=80?"strong":v>=60?"good":v>=40?"fair":"weak"; }

/* ---------- FitRing ---------- */
export function FitRing({ value, size=44, stroke, label }){
  const sw = stroke || Math.max(4, Math.round(size*0.11));
  const r = (size - sw) / 2;
  const c = 2 * Math.PI * r;
  const off = c * (1 - value/100);
  const big = size >= 96;
  return (
    <div className="kfit" data-band={fitBand(value)} style={{width:size, height:size}}>
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
        <circle className="kfit-track" cx={size/2} cy={size/2} r={r} fill="none" strokeWidth={sw}/>
        <circle className="kfit-val" cx={size/2} cy={size/2} r={r} fill="none" strokeWidth={sw}
          strokeDasharray={c} strokeDashoffset={off}/>
      </svg>
      <div className="kfit-num" style={{fontSize: size*0.34}}>
        {value}{big && label && <small>{label}</small>}
      </div>
    </div>
  );
}

/* ---------- StatusBadge ---------- */
export const STATUS_LABEL = { pending:"Pending", progress:"In Progress", done:"Done", human:"Needs Human", failed:"Failed" };
export function StatusBadge({ status, label }){
  return (
    <span className={`kbadge kbadge--${status}`}>
      <span className="dot"></span>{label || STATUS_LABEL[status]}
    </span>
  );
}

/* ---------- RecPill ---------- */
export function RecPill({ rec }){
  const map = {
    bid:   { c:"bid",    t:"Bid",    ic:<I.check width={13} height={13}/> },
    nobid: { c:"nobid",  t:"No Bid", ic:<svg viewBox="0 0 24 24" width={13} height={13} fill="none"><path d="M6 6l12 12M18 6L6 18" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round"/></svg> },
    review:{ c:"review", t:"Review", ic:<I.warn width={13} height={13}/> },
  };
  const m = map[rec] || map.review;
  return <span className={`krec krec--${m.c}`}>{m.ic}{m.t}</span>;
}

/* ---------- DeadlinePill ---------- */
export function DeadlinePill({ label, level="calm" }){
  const cls = level==="calm" ? "" : `kdead--${level}`;
  return <span className={`kdead ${cls}`}><I.clock width={13} height={13}/>{label}</span>;
}

/* ---------- Avatar (agent teammate) ---------- */
export const HUE = {
  blue:   { bg:"linear-gradient(155deg,#5B9BFF,#2563EB)", fg:"#fff" },
  cyan:   { bg:"linear-gradient(155deg,#67E0F4,#0EA5C4)", fg:"#062a33" },
  violet: { bg:"linear-gradient(155deg,#A99BFF,#7C6BF5)", fg:"#fff" },
};
export function Avatar({ agent, size="md", working=false }){
  const h = HUE[agent.hue] || HUE.blue;
  const cls = size==="lg" ? "kava kava--lg" : size==="sm" ? "kava kava--sm" : "kava";
  return (
    <span className={cls} style={{background:h.bg, color:h.fg}} data-working={working?"1":undefined}>
      {agent.initial}
      {working && <span className="kava-spin" aria-hidden="true"></span>}
    </span>
  );
}

/* ---------- Button ---------- */
export function Btn({ variant="secondary", size, icon, children, ...rest }){
  const sz = size==="lg" ? "kbtn--lg" : size==="sm" ? "kbtn--sm" : "";
  return (
    <button className={`kbtn kbtn--${variant} ${sz}`} {...rest}>
      {icon}{children}
    </button>
  );
}
