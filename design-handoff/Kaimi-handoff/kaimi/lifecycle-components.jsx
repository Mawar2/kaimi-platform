/* ============================================================
   KAIMI — shared React DS components + lifecycle state machine
   Exported to window for use by each Hero "take".
   ============================================================ */
const { useState, useEffect, useRef, useCallback } = React;

/* ---------- icons ---------- */
const I = {
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
function fitBand(v){ return v>=80?"strong":v>=60?"good":v>=40?"fair":"weak"; }

/* ---------- FitRing ---------- */
function FitRing({ value, size=44, stroke, label }){
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
const STATUS_LABEL = { pending:"Pending", progress:"In Progress", done:"Done", human:"Needs Human", failed:"Failed" };
function StatusBadge({ status, label }){
  return (
    <span className={`kbadge kbadge--${status}`}>
      <span className="dot"></span>{label || STATUS_LABEL[status]}
    </span>
  );
}

/* ---------- RecPill ---------- */
function RecPill({ rec }){
  const map = {
    bid:   { c:"bid",    t:"Bid",    ic:<I.check width={13} height={13}/> },
    nobid: { c:"nobid",  t:"No Bid", ic:<svg viewBox="0 0 24 24" width={13} height={13} fill="none"><path d="M6 6l12 12M18 6L6 18" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round"/></svg> },
    review:{ c:"review", t:"Review", ic:<I.warn width={13} height={13}/> },
  };
  const m = map[rec] || map.review;
  return <span className={`krec krec--${m.c}`}>{m.ic}{m.t}</span>;
}

/* ---------- DeadlinePill ---------- */
function DeadlinePill({ label, level="calm" }){
  const cls = level==="calm" ? "" : `kdead--${level}`;
  return <span className={`kdead ${cls}`}><I.clock width={13} height={13}/>{label}</span>;
}

/* ---------- Avatar (agent teammate) ---------- */
const HUE = {
  blue:   { bg:"linear-gradient(155deg,#5B9BFF,#2563EB)", fg:"#fff" },
  cyan:   { bg:"linear-gradient(155deg,#67E0F4,#0EA5C4)", fg:"#062a33" },
  violet: { bg:"linear-gradient(155deg,#A99BFF,#7C6BF5)", fg:"#fff" },
};
function Avatar({ agent, size="md", working=false }){
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
function Btn({ variant="secondary", size, icon, children, ...rest }){
  const sz = size==="lg" ? "kbtn--lg" : size==="sm" ? "kbtn--sm" : "";
  return (
    <button className={`kbtn kbtn--${variant} ${sz}`} {...rest}>
      {icon}{children}
    </button>
  );
}

/* ============================================================
   useLifecycle — the agent state machine
   ============================================================ */
function clock(){
  const d = new Date();
  return d.toLocaleTimeString("en-US", { hour12:false });
}
function useLifecycle(speed=1){
  const stages = window.KAIMI_STAGES;
  const idx = useCallback((id)=> stages.findIndex(s=>s.id===id), [stages]);
  const [statuses, setStatuses] = useState(()=> stages.map((s,i)=> i===0 ? "progress" : "pending"));
  const [phase, setPhase] = useState("running"); // running | gate | resuming | complete
  const [log, setLog] = useState([]);
  const timers = useRef([]);
  const startedRef = useRef(false);

  const clear = () => { timers.current.forEach(clearTimeout); timers.current = []; };
  const setStatus = (id, st) => setStatuses(prev => { const n=[...prev]; n[idx(id)]=st; return n; });
  const addLog = (e) => setLog(prev => [...prev, { ...e, time: clock() }]);
  const run = (steps) => {
    let t = 0;
    steps.forEach(s => { t += s.at/speed; timers.current.push(setTimeout(s.do, t)); });
  };

  const playIntro = useCallback(() => {
    clear();
    setStatuses(stages.map((s,i)=> i===0 ? "progress" : "pending"));
    setPhase("running");
    setLog([{ kind:"start", agent:"outline", text:"Noa started the outline", time:clock() }]);
    run([
      { at: 2000, do: ()=>{ setStatus("outline","done"); addLog({kind:"done", agent:"outline", text:"Noa mapped 7 sections to 24 requirements"});
                            setStatus("writer","progress"); addLog({kind:"start", agent:"writer", text:"Tomás started drafting the technical volume"}); } },
      { at: 2600, do: ()=>{ setStatus("writer","done"); addLog({kind:"done", agent:"writer", text:"Tomás drafted 18 pages across 7 sections"});
                            addLog({kind:"flag", agent:"writer", text:"Gap flagged — no past-performance for cybersecurity at scale"});
                            setStatus("gate","human"); setPhase("gate");
                            addLog({kind:"human", text:"Paused — waiting on your review"}); } },
    ]);
  }, []);

  useEffect(()=>{ if(!startedRef.current){ startedRef.current=true; const t=setTimeout(playIntro, 500); return ()=>clearTimeout(t);} }, [playIntro]);
  useEffect(()=> ()=>clear(), []);

  const approve = useCallback(() => {
    clear();
    setStatus("gate","done"); setPhase("resuming");
    setStatus("review","progress");
    addLog({kind:"approve", text:"You approved the draft — final pass resumed"});
    addLog({kind:"start", agent:"review", text:"Vera started the final compliance pass"});
    run([
      { at: 2400, do: ()=>{ setStatus("review","done"); addLog({kind:"done", agent:"review", text:"Vera cleared all 24 requirements — package is ready"});
                            setStatus("submit","done"); setPhase("complete");
                            addLog({kind:"ready", text:"Ready to submit to SAM.gov"}); } },
    ]);
  }, []);

  const requestChanges = useCallback(() => {
    clear();
    setStatus("gate","pending"); setStatus("writer","progress"); setPhase("running");
    addLog({kind:"changes", text:"You requested changes — sent back to Tomás"});
    addLog({kind:"start", agent:"writer", text:"Tomás is revising the technical volume"});
    run([
      { at: 2600, do: ()=>{ setStatus("writer","done"); addLog({kind:"done", agent:"writer", text:"Tomás revised the draft and resolved 1 gap"});
                            setStatus("gate","human"); setPhase("gate");
                            addLog({kind:"human", text:"Paused again — waiting on your review"}); } },
    ]);
  }, []);

  const restart = useCallback(()=>{ playIntro(); }, [playIntro]);

  // currentIndex: the active (progress/human) stage, else last done
  let currentIndex = statuses.findIndex(s=> s==="progress" || s==="human");
  if(currentIndex < 0){ for(let i=statuses.length-1;i>=0;i--){ if(statuses[i]==="done"){ currentIndex=i; break; } } }
  if(currentIndex < 0) currentIndex = 0;

  return { stages, statuses, phase, log, currentIndex, atGate: phase==="gate", approve, requestChanges, restart };
}

/* expose */
Object.assign(window, { I, FitRing, StatusBadge, RecPill, DeadlinePill, Avatar, Btn, useLifecycle, fitBand, STATUS_LABEL, HUE });
