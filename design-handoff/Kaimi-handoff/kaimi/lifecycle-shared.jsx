/* ============================================================
   KAIMI — shared composite pieces for the Hero takes
   ============================================================ */
const P = window.KAIMI_PROPOSAL;
const AG = window.KAIMI_AGENTS;

/* ---------- Proposal header ---------- */
function ProposalHeader(){
  return (
    <header className="prop-head">
      <FitRing value={P.fit} size={68} label="FIT" />
      <div className="htext">
        <span className="selected"><span className="live"></span>{P.selectedAgo}</span>
        <h1>{P.title}</h1>
        <div className="sub">
          <span>{P.agency}</span><span className="sep"></span>
          <span className="ktag">NAICS {P.naics}</span>
          <span className="ktag">SOL# {P.sol}</span>
        </div>
      </div>
      <div className="hmeta">
        <div className="stat"><div className="lab">Recommendation</div><div className="val" style={{marginTop:6}}><RecPill rec={P.rec}/></div></div>
        <div className="stat"><div className="lab">Deadline</div><div className="val" style={{marginTop:6}}><DeadlinePill label={P.deadlineLabel} level={P.deadlineLevel}/></div></div>
        <div className="stat"><div className="lab">Value</div><div className="val mono" style={{fontSize:13}}>$4.8M</div></div>
      </div>
    </header>
  );
}

/* ---------- Pipeline stepper (horizontal) ---------- */
function pipeIcon(st){
  if(st==="done") return <I.check/>;
  if(st==="progress") return <I.spinner/>;
  if(st==="human") return <I.hand/>;
  return <I.dot style={{opacity:.5}}/>;
}
function PipelineStepper({ statuses, onPick, currentIndex }){
  const stages = window.KAIMI_STAGES;
  const stLabel = (st)=> st==="human" ? "Needs you" : st==="progress" ? "Working" : st==="done" ? "Done" : "Pending";
  return (
    <div className="pl-pipe">
      {stages.map((s, i) => {
        const st = statuses[i];
        const prev = statuses[i-1];
        const connState = st==="progress" ? "flow" : (prev==="done" && (st==="done"||st==="human")) ? "done" : (prev==="done"?"done":"idle");
        return (
          <React.Fragment key={s.id}>
            {i>0 && <div className="pl-conn" data-on={
              statuses[i-1]==="done" && (statuses[i]==="progress") ? "flow"
              : statuses[i-1]==="done" ? "done" : "idle"
            }></div>}
            <div className="pl-node" data-st={st} onClick={()=>onPick && onPick(i)} style={{cursor:onPick?"pointer":"default"}}>
              <div className="pl-ring">
                {s.kind==="gate" && st!=="human" ? <I.star style={{opacity: st==="done"?1:.5}}/> : pipeIcon(st)}
              </div>
              <div className="pl-name">{s.name}</div>
              <div className="pl-state">{stLabel(st)}</div>
            </div>
          </React.Fragment>
        );
      })}
    </div>
  );
}

/* ---------- Artifacts / Metrics / Flag ---------- */
function Artifacts({ items }){
  if(!items || !items.length) return null;
  return (
    <div className="artifacts">
      {items.map((a,i)=>(
        <a className="artifact" key={i} onClick={(e)=>e.preventDefault()} href="#">
          <I.doc className="ic"/>
          <span>{a.name}</span>
          <span className="meta">{a.meta}</span>
          <I.link className="ext"/>
        </a>
      ))}
    </div>
  );
}
function Metrics({ items }){
  if(!items || !items.length) return null;
  return (
    <div className="metrics">
      {items.map((m,i)=>(<div className="metric" key={i}><div className="mv">{m.v}</div><div className="mk">{m.k}</div></div>))}
    </div>
  );
}
function FlagCard({ flag }){
  return (
    <div className="flag-card">
      <div className="fic"><I.warn/></div>
      <div>
        <div className="ftitle">{flag.title}</div>
        <div className="fdetail">{flag.detail}</div>
        <div className="faction"><Btn variant="changes" size="sm" icon={<I.link width={14} height={14}/>}>{flag.action}</Btn></div>
      </div>
    </div>
  );
}

/* ---------- Gate handoff (agent → you) ---------- */
function GateHandoff({ agent }){
  return (
    <div className="handoff">
      <div className="agentcol">
        <Avatar agent={agent} size="lg" />
        <span className="lbl">{agent.name}</span>
      </div>
      <div className="ho-arrow">
        <svg viewBox="0 0 30 18" fill="none">
          <path d="M2 9h22M19 3l6 6-6 6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" opacity="0.4"/>
          <circle className="pass" cx="8" cy="9" r="3" fill="currentColor"/>
        </svg>
      </div>
      <div className="youcol">
        <div className="you"><I.hand/></div>
        <span className="lbl">&nbsp;</span>
      </div>
    </div>
  );
}

/* ---------- Criteria checklist ---------- */
function CriteriaList({ items }){
  return (
    <div className="crit">
      {items.map((c,i)=>(
        <div className={`crit-item ${c.state}`} key={i}>
          <div className="ci-ic">{c.state==="ok" ? <I.check/> : <I.warn/>}</div>
          <div>
            <div className="ci-label">{c.label}</div>
            {c.note && <div className="ci-note">{c.note}</div>}
          </div>
        </div>
      ))}
    </div>
  );
}

/* ---------- Draft preview ---------- */
function DraftPreview(){
  const writer = window.KAIMI_STAGES.find(s=>s.id==="writer");
  return (
    <div className="draft-prev">
      <div className="dp-bar"><I.doc/><span>technical-volume-draft-v3.docx</span><span className="pages">18 pp</span></div>
      <div className="dp-page">
        <div className="dp-h"></div>
        <div className="dp-l" style={{width:"96%"}}></div>
        <div className="dp-l" style={{width:"90%"}}></div>
        <div className="dp-l" style={{width:"93%"}}></div>
        <div className="dp-l" style={{width:"40%"}}></div>
        <div className="dp-h" style={{width:"42%", marginTop:16}}></div>
        <div className="dp-l" style={{width:"94%"}}></div>
        <div className="dp-l" style={{width:"88%"}}></div>
      </div>
    </div>
  );
}

/* ---------- Gate actions ---------- */
function GateActions({ onApprove, onChanges }){
  return (
    <div className="gate-actions">
      <Btn variant="approve" size="lg" icon={<I.check width={18} height={18}/>} onClick={onApprove}>Approve &amp; resume</Btn>
      <Btn variant="changes" size="lg" icon={<I.back width={17} height={17}/>} onClick={onChanges}>Request changes</Btn>
      <div className="ga-note">Approving resumes Vera's final pass. Requesting changes sends the draft back to Tomás.</div>
    </div>
  );
}

/* ---------- Done banner ---------- */
function DoneBanner({ onRestart }){
  return (
    <div className="done-banner">
      <div className="db-ic"><I.check width={26} height={26}/></div>
      <div>
        <h2>Package ready to submit</h2>
        <p>All stages complete · 24/24 requirements addressed · validated for CISA format. Final human submission to SAM.gov.</p>
      </div>
      <div className="db-act">
        <Btn variant="ghost" size="lg" onClick={onRestart}>Replay</Btn>
        <Btn variant="select" size="lg" icon={<I.arrow width={18} height={18}/>}>Submit to SAM.gov</Btn>
      </div>
    </div>
  );
}

Object.assign(window, { ProposalHeader, PipelineStepper, Artifacts, Metrics, FlagCard, GateHandoff, CriteriaList, DraftPreview, GateActions, DoneBanner, pipeIcon });
