import React from 'react';
import { I, DeadlinePill } from './shared.jsx';
import { MiniPipe } from './screens.jsx';
import { KAIMI_STAGE_NAMES } from './data.js';

/* ============================================================
   KAIMI — App screen · Proposals (the command view)
   "Across everything, what needs me?"
   ============================================================ */

function ProposalCard({ p, onOpen, offline }){
  const stageName = KAIMI_STAGE_NAMES[p.stageIndex];
  const submitted = p.status==="submitted";
  const queued = p.status==="queued";
  return (
    <div className={`pcard ${p.status==="human"?"needs":""}`} role="button" tabIndex={0}
      onClick={()=>onOpen(p)}
      onKeyDown={(e)=>{ if(e.key==="Enter"||e.key===" "){ e.preventDefault(); onOpen(p); } }}>
      <div className="pc-body">
        <div className="pc-ttl">{p.title}</div>
        <div className="pc-agency">{p.agency} · {p.when}</div>
      </div>
      <div className="pc-pipe">
        <MiniPipe stageIndex={p.stageIndex} status={(submitted||queued)?"done":p.status} />
        <div className={`stage-label ${p.status==="human"?"human":""}`}>
          {p.status==="human" ? "Human Review" : submitted ? "Submitted to SAM.gov" : queued ? (p._queued==="changes"?"Changes queued · syncs online":"Approved · syncs online") : p.status==="done" ? "Ready to submit" : `${stageName}${p.agents?` · ${p.agents} agents`:""}`}
        </div>
      </div>
      <div className="pc-right">
        {p.status==="human"
          ? <span className="needs-tag"><I.hand/>Needs you</span>
          : submitted
          ? <span className="kbadge kbadge--done"><span className="dot"></span>Submitted</span>
          : queued
          ? <span className="kbadge kbadge--pending"><span className="dot"></span>Queued</span>
          : p.status==="done"
          ? <span className="kbadge kbadge--done"><span className="dot"></span>Ready</span>
          : offline
          ? <span className="kbadge kbadge--pending"><span className="dot"></span>Paused</span>
          : <span className="pc-working"><span className="pulse"></span>Working</span>}
        <DeadlinePill label={p.deadlineLabel} level={(submitted||queued)?"calm":p.deadlineLevel} />
        <span className="chev" style={{width:18,height:18,color:"var(--ink-4)"}}><svg viewBox="0 0 24 24" fill="none"><path d="M9 6l6 6-6 6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/></svg></span>
      </div>
    </div>
  );
}

function ProposalsScreen({ proposals, onOpen, offline }){
  const needs = proposals.filter(p=>p.status==="human");
  const queuedSync = proposals.filter(p=>p.status==="queued");
  const working = proposals.filter(p=>p.status==="progress");
  const ready = proposals.filter(p=>p.status==="done");
  const submitted = proposals.filter(p=>p.status==="submitted");
  const inFlight = proposals.length - submitted.length;
  const agentsTotal = proposals.reduce((s,p)=>s+(p.agents||0),0);

  return (
    <div className="page">
      <div className="page-head">
        <div className="eyebrow">Focus</div>
        <h1>Active proposals</h1>
        <p className="lead">Every proposal the agents are working right now — and, most importantly, the ones waiting on you.</p>
        <div className="stats">
          <div className="stat"><div className="v">{inFlight}<small> in flight</small></div><div className="k">Proposals being worked</div></div>
          <div className="stat"><div className="v">{agentsTotal}<small> agents</small></div><div className="k">Working across proposals</div></div>
          <div className="stat"><div className={`v ${needs.length?"amber":""}`}>{needs.length}<small> need you</small></div><div className="k">Paused at a review gate</div></div>
        </div>
      </div>

      {needs.length>0 && (
        <React.Fragment>
          <div className="section-h"><span className="lbl amber">Waiting on you</span><span className="cnt">{needs.length}</span><span className="ln"></span></div>
          <div className="prop-grid">{needs.map(p=><ProposalCard key={p.id} p={p} onOpen={onOpen} offline={offline} />)}</div>
        </React.Fragment>
      )}

      {queuedSync.length>0 && (
        <React.Fragment>
          <div className="section-h"><span className="lbl">Queued for sync</span><span className="cnt">{queuedSync.length}</span><span className="ln"></span></div>
          <div className="prop-grid">{queuedSync.map(p=><ProposalCard key={p.id} p={p} onOpen={onOpen} offline={offline} />)}</div>
        </React.Fragment>
      )}

      {working.length>0 && (
        <React.Fragment>
          <div className="section-h"><span className="lbl">{offline ? "Agents paused — resume online" : "Agents working"}</span><span className="cnt">{working.length}</span><span className="ln"></span></div>
          <div className="prop-grid">{working.map(p=><ProposalCard key={p.id} p={p} onOpen={onOpen} offline={offline} />)}</div>
        </React.Fragment>
      )}

      {ready.length>0 && (
        <React.Fragment>
          <div className="section-h"><span className="lbl">Ready to submit</span><span className="cnt">{ready.length}</span><span className="ln"></span></div>
          <div className="prop-grid">{ready.map(p=><ProposalCard key={p.id} p={p} onOpen={onOpen} />)}</div>
        </React.Fragment>
      )}

      {submitted.length>0 && (
        <React.Fragment>
          <div className="section-h"><span className="lbl">Submitted</span><span className="cnt">{submitted.length}</span><span className="ln"></span></div>
          <div className="prop-grid">{submitted.map(p=><ProposalCard key={p.id} p={p} onOpen={onOpen} />)}</div>
        </React.Fragment>
      )}

      {proposals.length===0 && (
        <div className="empty2"><div className="g"><svg width="26" height="26" viewBox="0 0 24 24" fill="none"><rect x="3" y="4" width="18" height="6" rx="2" stroke="currentColor" strokeWidth="2"/><rect x="3" y="14" width="18" height="6" rx="2" stroke="currentColor" strokeWidth="2"/></svg></div><h3>No active proposals</h3><p>Select an opportunity from your queue to spin up the agent workflow.</p></div>
      )}
    </div>
  );
}

export { ProposalsScreen };
