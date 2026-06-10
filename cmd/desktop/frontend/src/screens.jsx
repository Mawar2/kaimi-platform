import React from 'react';
import { FitRing, DeadlinePill, Btn, I } from './shared.jsx';
/* ============================================================
   KAIMI — App screens · Sidebar, Opportunities, Proposals
   Reuses FitRing, RecPill, DeadlinePill, Avatar, Btn, I (window).
   ============================================================ */
const NAVIC = {
  queue: <svg viewBox="0 0 24 24" fill="none"><path d="M4 6h16M4 12h16M4 18h10" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/></svg>,
  props: <svg viewBox="0 0 24 24" fill="none"><rect x="3" y="4" width="18" height="6" rx="2" stroke="currentColor" strokeWidth="2"/><rect x="3" y="14" width="18" height="6" rx="2" stroke="currentColor" strokeWidth="2"/></svg>,
  search: <svg viewBox="0 0 24 24" fill="none"><circle cx="11" cy="11" r="7" stroke="currentColor" strokeWidth="2"/><path d="M16 16l4 4" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/></svg>,
  sliders: <svg viewBox="0 0 24 24" fill="none"><path d="M4 8h10M18 8h2M4 16h2M10 16h10" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/><circle cx="16" cy="8" r="2.4" stroke="currentColor" strokeWidth="2"/><circle cx="8" cy="16" r="2.4" stroke="currentColor" strokeWidth="2"/></svg>,
  chev: <svg viewBox="0 0 24 24" fill="none"><path d="M9 6l6 6-6 6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/></svg>,
};

function Sidebar({ route, go, needsCount, queueCount, activeCount }){
  const item = (id, icon, label, extra) => (
    <button key={`${id}${route===id?"-on":""}`} className={`nav-item ${route===id?"on":""}`} onClick={()=>go(id)}>
      {icon}<span>{label}</span>{extra}
    </button>
  );
  return (
    <aside className="side">
      <div className="logo">
        <div className="mk">
          <svg width="22" height="22" viewBox="0 0 64 64" fill="none">
            <circle cx="45" cy="19" r="7" fill="#22D3EE"/>
            <path d="M9 38C17 28 24 28 31 38C38 48 45 48 53 38" stroke="#67E0F4" strokeWidth="5.4" strokeLinecap="round"/>
            <path d="M9 48C17 38 24 38 31 48C38 58 45 58 53 48" stroke="#fff" strokeWidth="5.4" strokeLinecap="round" opacity="0.9"/>
          </svg>
        </div>
        <div className="nm">Kaimi<small>the seeker</small></div>
      </div>

      <div className="nav-h">Pipeline</div>
      {item("opps", NAVIC.queue, "Opportunities", <span className="count">{queueCount}</span>)}
      {item("proposals", NAVIC.props, "Proposals", needsCount>0 ? <span className="needs">{needsCount}</span> : <span className="count">{activeCount}</span>)}

      <div className="spacer"></div>
      <div className="me">
        <div className="av">BM</div>
        <div className="who"><b>BlueMeta BD</b><span>Captures team</span></div>
      </div>
    </aside>
  );
}

/* ---------- mini pipeline (dots) ---------- */
function MiniPipe({ stageIndex, status }){
  const n = 5;
  const nodeSt = (i) => i < stageIndex ? "done" : i === stageIndex ? (status==="human" ? "human" : status==="done" ? "done" : "active") : "";
  return (
    <div className="minipipe">
      {Array.from({length:n}).map((_,i)=>(
        <React.Fragment key={i}>
          {i>0 && <div className={`seg ${i<=stageIndex ? "done" : ""}`}></div>}
          <div className={`node ${nodeSt(i)}`}></div>
        </React.Fragment>
      ))}
    </div>
  );
}

/* ============================================================
   OPPORTUNITIES (Triage queue)
   ============================================================ */
function OppRow({ opp, onOpen }){
  return (
    <div className={`orow ${opp.isNew?"new":""}`} role="button" tabIndex={0}
      onClick={()=>onOpen(opp)}
      onKeyDown={(e)=>{ if(e.key==="Enter"||e.key===" "){ e.preventDefault(); onOpen(opp); } }}>
      <span className="newdot" title="New today"></span>
      <FitRing value={opp.fit} size={46} />
      <div className="body">
        <div className="ttl">{opp.title}</div>
        <div className="meta">
          <span>{opp.agency}</span><span className="sep"></span>
          <span className="naics">NAICS {opp.naics}</span>
        </div>
      </div>
      <div className="right">
        <span className={`rec-min rec-min--${opp.rec}`}>{opp.rec==="nobid"?"No bid":opp.rec}</span>
        <DeadlinePill label={opp.deadlineLabel} level={opp.deadlineLevel} />
        <span className="chev">{NAVIC.chev}</span>
      </div>
    </div>
  );
}

function OpportunitiesScreen({ opps, onOpen, filter, setFilter }){
  const filtered = opps.filter(o =>
    filter==="all" ? true : filter==="bid" ? o.rec==="bid" : o.rec==="review"
  );
  const newOnes = filtered.filter(o=>o.day==="today");
  const earlier = filtered.filter(o=>o.day!=="today");
  const newCount = opps.filter(o=>o.isNew).length;
  const topFit = Math.max(...opps.map(o=>o.fit));

  return (
    <div className="page">
      <div className="page-head">
        <div className="eyebrow">Triage</div>
        <h1>Opportunities</h1>
        <p className="lead">Live federal opportunities Kaimi hunted and scored against your capabilities. Pick what to pursue.</p>
        <div className="stats">
          <div className="stat"><div className="v">{opps.length}<small> in queue</small></div><div className="k">From last night's SAM.gov run</div></div>
          <div className="stat"><div className="v">{newCount}<small> new</small></div><div className="k">Added today</div></div>
          <div className="stat"><div className="v">{topFit}</div><div className="k">Top fit score</div></div>
        </div>
      </div>

      <div className="toolbar">
        <div className="seg">
          <button key={`all${filter==="all"?"-on":""}`} className={filter==="all"?"on":""} onClick={()=>setFilter("all")}>All</button>
          <button key={`bid${filter==="bid"?"-on":""}`} className={filter==="bid"?"on":""} onClick={()=>setFilter("bid")}>To pursue</button>
          <button key={`review${filter==="review"?"-on":""}`} className={filter==="review"?"on":""} onClick={()=>setFilter("review")}>Needs review</button>
        </div>
        <div className="grow"></div>
        <button className="sortbtn">{NAVIC.sliders}Sort: Fit score</button>
      </div>

      <div className="opp-list">
        {newOnes.length>0 && <div className="day"><span>New today</span><span className="ln"></span></div>}
        {newOnes.map(o=><OppRow key={o.id} opp={o} onOpen={onOpen} />)}
        {earlier.length>0 && <div className="day"><span>Earlier this week</span><span className="ln"></span></div>}
        {earlier.map(o=><OppRow key={o.id} opp={o} onOpen={onOpen} />)}
        {filtered.length===0 && (
          <div className="empty2"><div className="g">{NAVIC.search}</div><h3>Nothing here right now</h3><p>No opportunities match this filter. The next hunt runs tonight at 02:00.</p></div>
        )}
      </div>
    </div>
  );
}

/* ---------- Opportunity detail drawer (the pursue decision) ---------- */
function OppDrawer({ opp, onClose, onSelect, pursued, offline }){
  React.useEffect(()=>{
    const onKey = (e)=>{ if(e.key==="Escape") onClose(); };
    window.addEventListener("keydown", onKey);
    return ()=>window.removeEventListener("keydown", onKey);
  }, [onClose]);
  if(!opp) return null;
  const reasons = opp.rec==="bid"
    ? [`Strong NAICS match (${opp.naics}) against your core capabilities.`,
       "Recent past performance aligns with the technical scope.",
       "Scope and ceiling fit your typical award size."]
    : opp.rec==="review"
    ? [`NAICS ${opp.naics} is adjacent but not core to your past work.`,
       "Past-performance evidence is thin for this scope — may need a partner.",
       "Timeline is tight relative to a from-scratch proposal."]
    : ["Outside your core NAICS and capability set.",
       "Past performance does not support the evaluation factors.",
       "Margin and fit do not justify the pursuit cost."];
  const musts = [
    { t:"Active facility clearance at the required level", ok:true },
    { t:"Two relevant references within 5 years", ok: opp.rec!=="nobid" },
    { t:"Key personnel with required certifications", ok:true },
    { t:"Past performance at comparable contract value", ok: opp.fit>=75 },
  ];
  return (
    <div className="drawer-scrim" onClick={onClose}>
      <div className="drawer" onClick={e=>e.stopPropagation()}>
        <div className="dr-head">
          <button className="dr-close" onClick={onClose}><I.back/></button>
          <span className={`rec-min rec-min--${opp.rec}`} style={{fontSize:13}}>{opp.rec==="nobid"?"No bid":opp.rec==="bid"?"Recommend bid":"Needs review"}</span>
          <DeadlinePill label={opp.deadlineLabel} level={opp.deadlineLevel} />
        </div>
        <div className="dr-body">
          <div className="dr-top">
            <FitRing value={opp.fit} size={92} label="FIT" />
            <div>
              <h2>{opp.title}</h2>
              <div className="dr-sub">{opp.agency}{opp.value ? <span> · <b style={{color:"var(--ink)",fontWeight:600}}>{opp.value}</b></span> : null}</div>
              <div className="dr-tags">
                <span className="ktag">NAICS {opp.naics}</span>
                <span className="ktag">SOL# {opp.sol}</span>
              </div>
            </div>
          </div>

          <div className="dr-sec-h">Why Kaimi scored this {opp.fit}</div>
          <ul className="reasons">
            {reasons.map((r,i)=><li key={i}><span className="rd"></span>{r}</li>)}
          </ul>

          <div className="dr-sec-h">Must-have requirements</div>
          <div className="musts">
            {musts.map((m,i)=>(
              <div className={`must ${m.ok?"ok":"no"}`} key={i}>
                <span className="mc">{m.ok ? <I.check/> : <I.warn/>}</span>{m.t}
              </div>
            ))}
          </div>
        </div>
        <div className="dr-actions">
          <a className="artifact2" href="#" onClick={e=>e.preventDefault()}><I.link/>View solicitation</a>
          <div style={{flex:1}}></div>
          {pursued
            ? <Btn variant="secondary" disabled><I.check width={16} height={16}/>In your proposals</Btn>
            : offline
            ? <Btn variant="secondary" disabled>Reconnect to pursue</Btn>
            : <Btn variant="select" size="lg" icon={<I.arrow width={18} height={18}/>} onClick={()=>onSelect(opp)}>Select to pursue</Btn>}
        </div>
      </div>
    </div>
  );
}

export { Sidebar, MiniPipe, OpportunitiesScreen, OppDrawer };
