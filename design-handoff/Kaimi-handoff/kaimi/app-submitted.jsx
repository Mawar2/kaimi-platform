/* ============================================================
   KAIMI — App screen · Submitted (the pipeline archive)
   "What's out the door, what's it worth, and where's everything
    we produced for it?"
   + Record award outcomes  + Export BD report (CSV)
   ============================================================ */

const SUB_STATUS = {
  pending: { label:"Pending award", cls:"kbadge--pending" },
  won:     { label:"Won",           cls:"kbadge--done" },
  lost:    { label:"Not awarded",   cls:"kbadge--muted" },
};

function fmtM(n){
  if(n >= 1) { const s = n.toFixed(1); return "$" + (s.endsWith(".0") ? s.slice(0,-2) : s) + "M"; }
  return "$" + Math.round(n*1000) + "K";
}

/* Federal fiscal year quarter (FY starts Oct 1). */
function fyQuarter(d){
  const m = d.getMonth();
  const fy = d.getFullYear() + (m >= 9 ? 1 : 0);
  const q = m >= 9 ? 1 : m <= 2 ? 2 : m <= 5 ? 3 : 4;
  return { fy, q, label: "FY" + String(fy).slice(2) + " Q" + q };
}
function subDate(s){
  const d = s.isNew ? new Date() : new Date(s.submitted);
  return isNaN(d) ? new Date() : d;
}

function SubmittedRow({ s, open, onToggle, onOutcome }){
  const st = SUB_STATUS[s.status] || SUB_STATUS.pending;
  const seg = (val, label) => (
    <button key={`${val}${s.status===val?"-on":""}`} className={s.status===val?"on":""}
      onClick={()=>onOutcome(s.id, val)}>{label}</button>
  );
  return (
    <div className={`srow ${open?"open":""} ${s.isNew?"justnow":""}`}>
      <div className="srow-head" role="button" tabIndex={0} aria-expanded={open}
        onClick={onToggle}
        onKeyDown={(e)=>{ if(e.key==="Enter"||e.key===" "){ e.preventDefault(); onToggle(); } }}>
        <FitRing value={s.fit} size={42} />
        <div className="s-body">
          <div className="sttl">{s.title}</div>
          <div className="smeta">
            <span>{s.agency}</span><span className="sep"></span>
            <span className="mono">SOL# {s.sol}</span><span className="sep"></span>
            <span>Submitted {s.submitted}</span>
          </div>
        </div>
        <div className="s-right">
          <span className="sval">{s.value}</span>
          <span className={`kbadge ${st.cls}`}><span className="dot"></span>{st.label}</span>
          <span className={`schev ${open?"up":""}`}>
            <svg viewBox="0 0 24 24" fill="none"><path d="M6 9l6 6 6-6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/></svg>
          </span>
        </div>
      </div>
      {open && (
        <div className="srow-docs">
          <div className="sd-note">{s.award}</div>
          <div className="sd-grid">
            <div>
              <div className="sd-h">Reference documents</div>
              <div className="art-row" style={{marginTop:8}}>
                {s.docs.map((d,i)=>(
                  <a className="artifact2" key={i} href="#" onClick={e=>e.preventDefault()}>
                    <I.doc/>{d.name}
                    <span style={{color:"var(--ink-4)",fontFamily:"var(--font-mono)",fontSize:11}}>{d.meta}</span>
                  </a>
                ))}
              </div>
            </div>
            <div className="sd-outcome">
              <div className="sd-h">Outcome</div>
              <div className="seg" style={{marginTop:8}}>
                {seg("pending","Pending")}
                {seg("won","Won")}
                {seg("lost","Not awarded")}
              </div>
              <div className="sd-hint">Award decisions update your pipeline stats. Kaimi also watches SAM.gov for award notices.</div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

/* ---------- Export BD report ---------- */
function ExportDialog({ items, onClose }){
  const now = new Date();
  const toQI = (d)=>{ const f = fyQuarter(d); return f.fy*4 + (f.q-1); };
  const qiLabel = (qi)=> "FY" + String(Math.floor(qi/4)).slice(2) + " Q" + (qi%4+1);
  const curQI = toQI(now);
  const dataQIs = new Set(items.map(s=> toQI(subDate(s))));
  const minQI = Math.min(curQI, ...items.map(s=> toQI(subDate(s))));
  /* only quarters that actually contain submissions are offered */
  const quarters = [];
  for(let qi=minQI; qi<=curQI; qi++) if(dataQIs.has(qi)) quarters.push(qi);
  const latestDataQI = quarters.length ? quarters[quarters.length-1] : curQI;

  /* range = [startQI, endQI]; click one quarter, or a start then an end */
  const initQI = dataQIs.has(curQI) ? curQI : latestDataQI;
  const [range, setRange] = React.useState([initQI, initQI]);
  const [anchor, setAnchor] = React.useState(null);
  const pick = (qi)=>{
    if(anchor==null){ setAnchor(qi); setRange([qi,qi]); }
    else{ setRange([Math.min(anchor,qi), Math.max(anchor,qi)]); setAnchor(null); }
  };
  const fyStart = Math.max(Math.floor(curQI/4)*4, minQI);
  const hasData = (a,b)=> quarters.some(qi=> qi>=a && qi<=b);
  const PRESETS = [
    { label:"This quarter", a:curQI, b:curQI },
    { label:"FY" + String(Math.floor(curQI/4)).slice(2) + " to date", a:fyStart, b:curQI },
    { label:"All time", a:minQI, b:curQI },
  ].filter(p=> hasData(p.a, p.b));
  const setPreset = (p)=>{ setRange([p.a, p.b]); setAnchor(null); };
  const isPreset = (p)=> range[0]===p.a && range[1]===p.b;

  const rangeLabel = (()=>{
    const [a,b] = range;
    if(a===b) return qiLabel(a);
    if(Math.floor(a/4)===Math.floor(b/4)) return qiLabel(a) + "\u2013Q" + (b%4+1);
    return qiLabel(a) + " \u2013 " + qiLabel(b);
  })();

  const rows = items.filter(s=>{ const qi = toQI(subDate(s)); return qi>=range[0] && qi<=range[1]; });

  const totalVal = rows.reduce((a,s)=>a+(s.valueNum||0),0);
  const won = rows.filter(s=>s.status==="won");
  const lost = rows.filter(s=>s.status==="lost");
  const wonVal = won.reduce((a,s)=>a+(s.valueNum||0),0);
  const decided = won.length + lost.length;
  const winRate = decided ? Math.round(won.length/decided*100) + "%" : "—";

  const download = ()=>{
    const esc = (v)=> '"' + String(v).replace(/"/g,'""') + '"';
    const lines = [
      ["Kaimi BD report", rangeLabel],
      ["Generated", now.toLocaleDateString("en-US",{month:"short",day:"numeric",year:"numeric"})],
      [],
      ["Proposals submitted", rows.length],
      ["Total submitted value ($M)", totalVal.toFixed(2)],
      ["Won value ($M)", wonVal.toFixed(2)],
      ["Win rate (decided)", winRate],
      [],
      ["Title","Agency","Solicitation","Submitted","FY quarter","Value ($M)","Status"],
      ...rows.map(s=>[ s.title, s.agency, s.sol, s.isNew?"Today":s.submitted, qiLabel(toQI(subDate(s))), (s.valueNum||0).toFixed(2), SUB_STATUS[s.status].label ]),
    ].map(r=>r.map(esc).join(",")).join("\n");
    const blob = new Blob([lines], {type:"text/csv"});
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = "kaimi-bd-report-" + rangeLabel.toLowerCase().replace(/[^a-z0-9]+/g,"-") + ".csv";
    a.click();
    setTimeout(()=>URL.revokeObjectURL(a.href), 4000);
  };

  React.useEffect(()=>{
    const onKey = (e)=>{ if(e.key==="Escape") onClose(); };
    window.addEventListener("keydown", onKey);
    return ()=>window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div className="xscrim" onClick={onClose}>
      <div className="xmodal" onClick={e=>e.stopPropagation()}>
        <h3>Export BD report</h3>
        <p>A CSV with headline metrics plus one row per submitted proposal — for planning, board decks, and bank conversations.</p>
        <div className="x-periods">
          {PRESETS.map((p,i)=>(
            <button key={`${i}${isPreset(p)?"-on":""}`} className={`x-period ${isPreset(p)?"on":""}`} onClick={()=>setPreset(p)}>{p.label}</button>
          ))}
        </div>
        <div className="x-qh">Or pick a range — click a start quarter, then an end <b>· {rangeLabel}</b></div>
        <div className="x-qstrip">
          {quarters.map(qi=>{
            const on = qi>=range[0] && qi<=range[1];
            return (
              <button key={`${qi}${on?"-on":""}${anchor===qi?"-a":""}`}
                className={`x-q ${on?"on":""} ${anchor===qi?"anchor":""}`} onClick={()=>pick(qi)}>
                {qiLabel(qi)}
              </button>
            );
          })}
        </div>
        <div className="x-metrics">
          <div className="xm"><b>{rows.length}</b><span>submitted</span></div>
          <div className="xm"><b>{fmtM(totalVal)}</b><span>total value</span></div>
          <div className="xm"><b>{fmtM(wonVal)}</b><span>won</span></div>
          <div className="xm"><b>{winRate}</b><span>win rate</span></div>
        </div>
        <div className="x-actions">
          <Btn variant="ghost" onClick={onClose}>Cancel</Btn>
          <Btn variant="select" icon={<svg width="16" height="16" viewBox="0 0 24 24" fill="none"><path d="M12 4v11m0 0l-4-4m4 4l4-4M5 20h14" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/></svg>} onClick={download}>Download CSV</Btn>
        </div>
      </div>
    </div>
  );
}

function SubmittedScreen({ items }){
  const [q, setQ] = React.useState("");
  const [filter, setFilter] = React.useState("all");
  const [openId, setOpenId] = React.useState(null);
  const [outcomes, setOutcomes] = React.useState({});
  const [exporting, setExporting] = React.useState(false);

  /* outcome overrides layer on top of the synced data */
  const eff = items.map(s => outcomes[s.id] ? {...s, status:outcomes[s.id],
    award: outcomes[s.id]==="won" ? "Marked won by you · just now" : outcomes[s.id]==="lost" ? "Marked not awarded by you · just now" : s.award } : s);
  const setOutcome = (id, status)=> setOutcomes(prev=>({ ...prev, [id]:status }));

  const pending = eff.filter(s=>s.status==="pending");
  const won = eff.filter(s=>s.status==="won");
  const pendingVal = pending.reduce((a,s)=>a+(s.valueNum||0),0);
  const wonVal = won.reduce((a,s)=>a+(s.valueNum||0),0);

  const ql = q.trim().toLowerCase();
  const shown = eff.filter(s=>{
    if(filter!=="all" && s.status!==filter) return false;
    if(!ql) return true;
    return (s.title+" "+s.agency+" "+s.sol).toLowerCase().includes(ql);
  });

  return (
    <div className="page">
      <div className="page-head">
        <div className="eyebrow">Pipeline</div>
        <h1>Submitted</h1>
        <p className="lead">Every proposal that's gone out the door — what it's worth, and everything the team produced along the way.</p>
        <div className="stats">
          <div className="stat"><div className="v">{fmtM(pendingVal)}<small> awaiting award</small></div><div className="k">{pending.length} proposals pending decision</div></div>
          <div className="stat"><div className="v">{fmtM(wonVal)}<small> won</small></div><div className="k">{won.length} awards in the last 12 months</div></div>
          <div className="stat"><div className="v">{eff.length}<small> submitted</small></div><div className="k">All time, via Kaimi</div></div>
        </div>
      </div>

      <div className="toolbar">
        <div className="searchbox">
          <svg viewBox="0 0 24 24" fill="none"><circle cx="11" cy="11" r="7" stroke="currentColor" strokeWidth="2"/><path d="M16 16l4 4" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/></svg>
          <input type="search" placeholder="Search by title, agency, or solicitation…" value={q} onChange={e=>setQ(e.target.value)} />
        </div>
        <div className="grow"></div>
        <div className="seg">
          <button key={`all${filter==="all"?"-on":""}`} className={filter==="all"?"on":""} onClick={()=>setFilter("all")}>All</button>
          <button key={`pending${filter==="pending"?"-on":""}`} className={filter==="pending"?"on":""} onClick={()=>setFilter("pending")}>Pending award</button>
          <button key={`won${filter==="won"?"-on":""}`} className={filter==="won"?"on":""} onClick={()=>setFilter("won")}>Won</button>
          <button key={`lost${filter==="lost"?"-on":""}`} className={filter==="lost"?"on":""} onClick={()=>setFilter("lost")}>Not awarded</button>
        </div>
        <button className="sortbtn" onClick={()=>setExporting(true)}>
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none"><path d="M12 4v11m0 0l-4-4m4 4l4-4M5 20h14" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/></svg>
          Export report
        </button>
      </div>

      <div className="sub-list">
        {shown.map(s=>(
          <SubmittedRow key={s.id} s={s} open={openId===s.id}
            onToggle={()=>setOpenId(openId===s.id?null:s.id)} onOutcome={setOutcome} />
        ))}
        {shown.length===0 && (
          <div className="empty2">
            <div className="g"><svg width="26" height="26" viewBox="0 0 24 24" fill="none"><circle cx="11" cy="11" r="7" stroke="currentColor" strokeWidth="2"/><path d="M16 16l4 4" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/></svg></div>
            <h3>No matches</h3>
            <p>{ql ? <span>Nothing in the archive matches "{q}".</span> : "Nothing here yet — submit a proposal and it lands in this archive."}</p>
          </div>
        )}
      </div>

      {exporting && <ExportDialog items={eff} onClose={()=>setExporting(false)} />}
    </div>
  );
}

window.SubmittedScreen = SubmittedScreen;
