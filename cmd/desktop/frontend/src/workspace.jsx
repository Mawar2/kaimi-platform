import React from 'react';
import { FitRing, Avatar, Btn, I, DeadlinePill, HUE } from './shared.jsx';
import { KAIMI_STAGE_NAMES, KAIMI_REVIEW, KAIMI_AGENTS } from './data.js';

/* ============================================================
   KAIMI — App screen · Workspace (calm single-proposal review)
   ============================================================ */

function WPipe({ stageIndex, status }){
  const names = KAIMI_STAGE_NAMES;
  const nodeSt = (i)=> i<stageIndex ? "done" : i===stageIndex ? (status==="human"?"human":(status==="done"||status==="submitted"||status==="queued")?"done":"progress") : "pending";
  const icon = (i, st)=>{
    if(i===2) return <I.hand/>;
    if(i===4) return st==="done" ? <I.check/> : <I.arrow/>;
    if(st==="done") return <I.check/>;
    if(st==="progress") return <I.spinner/>;
    return <I.dot style={{opacity:.5}}/>;
  };
  return (
    <div className="wpipe">
      {names.map((nm,i)=>{
        const st = nodeSt(i);
        return (
          <React.Fragment key={i}>
            {i>0 && <div className="wconn" data-on={i<=stageIndex?"done":"idle"}></div>}
            <div className="wnode" data-st={st}>
              <div className="wring">{icon(i,st)}</div>
              <div className="wname">{nm}</div>
              <div className="wstate">{st==="human"?"Needs you":st==="progress"?"Working":st==="done"?"Done":"Pending"}</div>
            </div>
          </React.Fragment>
        );
      })}
    </div>
  );
}

function ReviewCard({ onApprove, onChanges, onOpenDraft }){
  const R = KAIMI_REVIEW;
  const writer = KAIMI_AGENTS.writer;
  const hue = HUE[writer.hue];
  return (
    <div className="review">
      <div className="r-head">
        <span className="r-badge"><I.hand width={14} height={14}/>Needs you</span>
        <div>
          <h2>Tomás is handing you the draft</h2>
          <p>{R.prompt}</p>
        </div>
        <div className="r-hand">
          <span className="av" style={{background:hue.bg, color:hue.fg}}>{writer.initial}</span>
          <span className="arrow"><svg width="26" height="16" viewBox="0 0 26 16" fill="none"><path d="M2 8h20M17 3l5 5-5 5" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/></svg></span>
          <span className="you"><I.hand/></span>
        </div>
      </div>
      <div className="r-body">
        <div className="r-sec-h">What Tomás produced</div>
        <div className="summary">{R.summary}</div>
        <div className="art-row">
          {onOpenDraft && (
            <a className="artifact2" href="#" style={{borderColor:"var(--blue-300)", color:"var(--blue-700)", fontWeight:600}}
              onClick={e=>{e.preventDefault(); onOpenDraft();}}>
              <svg width="15" height="15" viewBox="0 0 24 24" fill="none"><path d="M4 20h16M14 4l6 6L9 21H4v-5z" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/></svg>
              Edit the draft
            </a>
          )}
          {R.artifacts.map((a,i)=>(
            <a className="artifact2" key={i} href="#" onClick={e=>e.preventDefault()}><I.doc/>{a.name}<span style={{color:"var(--ink-4)",fontFamily:"var(--font-mono)",fontSize:11}}>{a.meta}</span></a>
          ))}
        </div>

        <div className="gapflag">
          <div className="gf-ic"><I.warn/></div>
          <div>
            <div className="gf-t">{R.gap.title}</div>
            <div className="gf-d">{R.gap.detail}</div>
          </div>
        </div>

        <div className="r-sec-h" style={{marginTop:24}}>Check against criteria</div>
        <div className="crit2">
          {R.criteria.map((c,i)=>(
            <div className={`citem ${c.state}`} key={i}>
              <span className="ci-ic">{c.state==="ok"?<I.check/>:<I.warn/>}</span>
              <div><div className="ci-l">{c.label}</div>{c.note && <div className="ci-n">{c.note}</div>}</div>
            </div>
          ))}
        </div>
      </div>
      <div className="r-actions">
        <Btn variant="approve" size="lg" icon={<I.check width={18} height={18}/>} onClick={onApprove}>Approve &amp; resume</Btn>
        <Btn variant="changes" size="lg" icon={<I.back width={17} height={17}/>} onClick={onChanges}>Request changes</Btn>
        <div className="note">Approving resumes Vera's final pass. Requesting changes sends it back to Tomás.</div>
      </div>
    </div>
  );
}

function WorkingState({ agentKey, stageIndex }){
  const a = KAIMI_AGENTS[agentKey];
  const lines = {
    outline: "Mapping the solicitation into a section plan and matching each requirement to an evaluation factor.",
    writer:  "Drafting the technical volume from the outline — section by section, in CISA's required format.",
    review:  "Running the final compliance and consistency pass. Validating every requirement and cross-reference.",
  }[agentKey];
  return (
    <div className="ws-state">
      <Avatar agent={a} size="lg" working={true} />
      <div>
        <h3>{a.name} is working</h3>
        <div className="role">{a.role} agent · {KAIMI_STAGE_NAMES[stageIndex]}</div>
        <div className="desc">{lines}</div>
      </div>
    </div>
  );
}

function ReadyState({ onSubmit }){
  return (
    <div className="ws-state" style={{borderColor:"color-mix(in oklab,var(--st-done) 40%,transparent)", background:"linear-gradient(180deg,var(--st-done-bg),#fff 60%)"}}>
      <span className="ws-av" style={{background:"linear-gradient(155deg,#2BD49A,#15A06B)"}}><I.check width={24} height={24}/></span>
      <div style={{flex:1}}>
        <h3>Package ready to submit</h3>
        <div className="desc" style={{marginTop:8}}>All stages complete · 24/24 requirements addressed · validated for CISA format. Final human submission to SAM.gov.</div>
        <div style={{marginTop:16}}><Btn variant="select" size="lg" icon={<I.arrow width={18} height={18}/>} onClick={onSubmit}>Submit to SAM.gov</Btn></div>
      </div>
    </div>
  );
}

function SubmittedState(){
  return (
    <div className="ws-state" style={{borderColor:"color-mix(in oklab,var(--st-done) 40%,transparent)"}}>
      <span className="ws-av" style={{background:"linear-gradient(155deg,#2BD49A,#15A06B)"}}><I.check width={24} height={24}/></span>
      <div>
        <h3>Submitted to SAM.gov</h3>
        <div className="role">Confirmation logged · the agents stand down on this one.</div>
        <div className="desc">Kaimi will watch for amendments and Q&amp;A updates on this solicitation and let you know if anything needs attention.</div>
      </div>
    </div>
  );
}

function WorkspaceScreen({ p, onBack, onApprove, onChanges, onSubmit, onOpenDraft }){
  if(!p) return null;
  const agentKeyFor = (i)=> i===0?"outline":i===1?"writer":i===3?"review":"writer";
  return (
    <div className="ws">
      <button className="back" onClick={onBack}><I.back/>All proposals</button>
      <div className="ws-head">
        <FitRing value={p.fit} size={64} label="FIT" />
        <div className="ws-id">
          <h1>{p.title}</h1>
          <div className="ws-meta">
            <span>{p.agency}</span><span className="sep"></span>
            <DeadlinePill label={p.deadlineLabel} level={p.deadlineLevel} />
            <span className="sep"></span>
            <span>{p.status==="human" ? "Paused for your review" : p.status==="done" ? "Ready to submit" : p.status==="submitted" ? "Submitted" : p.status==="queued" ? "Queued — syncs when online" : `${KAIMI_STAGE_NAMES[p.stageIndex]} in progress`}</span>
          </div>
        </div>
      </div>

      <WPipe stageIndex={p.stageIndex} status={p.status} />

      {p.status==="human" && <ReviewCard onApprove={()=>onApprove(p)} onChanges={()=>onChanges(p)} onOpenDraft={onOpenDraft ? ()=>onOpenDraft(p) : undefined} />}
      {p.status==="progress" && <WorkingState agentKey={agentKeyFor(p.stageIndex)} stageIndex={p.stageIndex} />}
      {p.status==="done" && <ReadyState onSubmit={()=>onSubmit(p)} />}
      {p.status==="submitted" && <SubmittedState />}
      {p.status==="queued" && (
        <div className="ws-state">
          <span className="ws-av" style={{background:"var(--n-200)", color:"var(--ink-2)"}}>
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none"><circle cx="12" cy="13" r="8" stroke="currentColor" strokeWidth="2"/><path d="M12 9v4l3 2" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/></svg>
          </span>
          <div>
            <h3>{p._queued==="changes" ? "Changes requested — queued for sync" : "Approved — queued for sync"}</h3>
            <div className="role">Saved on this device while offline</div>
            <div className="desc">{p._queued==="changes" ? "Your notes are saved locally. When you're back online, the draft returns to Tomás with your direction." : "Your decision and any draft edits are saved locally. When you're back online, Vera's final pass starts automatically."}</div>
          </div>
        </div>
      )}
    </div>
  );
}

export { WorkspaceScreen };
