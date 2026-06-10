/* ============================================================
   KAIMI — Hero · TAKE B — "Focus Stage"
   A slim rail of stages + one large focus stage. When the gate
   is reached, the rail dims and the stage becomes a spotlight
   where the agent passes the draft to you. Calm, status-driven.
   ============================================================ */

function RailChip({ stage, status, active, onClick }){
  const agent = stage.agentKey ? window.KAIMI_AGENTS[stage.agentKey] : null;
  const sub = status==="human" ? "Needs you" : status==="progress" ? "Working" : status==="done" ? "Done" : "Pending";
  let icon;
  if(stage.kind==="gate") icon = <I.hand/>;
  else if(stage.kind==="terminal") icon = <I.arrow/>;
  else if(status==="done") icon = <I.check/>;
  else if(status==="progress") icon = <I.spinner/>;
  else icon = <I.dot style={{opacity:.5}}/>;
  return (
    <button className="tb-chip" data-st={status} data-active={active?"1":undefined} onClick={onClick}>
      <span className="tc-mk">{icon}</span>
      <span className="tc-info">
        <span className="tc-name">{stage.kind==="agent" ? agent.name : stage.name}</span>
        <span className="tc-sub">{stage.kind==="agent" ? stage.name : sub}</span>
      </span>
    </button>
  );
}

function FocusContent({ stage, status, lc }){
  const gate = window.KAIMI_STAGES.find(s=>s.id==="gate");
  const writer = window.KAIMI_STAGES.find(s=>s.id==="writer");

  if(stage.kind==="gate"){
    if(status==="human"){
      return (
        <React.Fragment>
          <div className="fs-gate-head">
            <div className="gate-badge"><I.hand width={14} height={14}/>Needs you</div>
            <div className="fg-text">
              <h2>Tomás is passing you the draft</h2>
              <p>{gate.prompt}</p>
            </div>
            <div style={{marginLeft:"auto"}}><GateHandoff agent={window.KAIMI_AGENTS.writer} /></div>
          </div>
          <div className="fs-gate-grid">
            <div>
              <h4 className="fs-block-h"><I.doc width={13} height={13} style={{verticalAlign:"-2px",marginRight:6}}/>The draft</h4>
              <DraftPreview />
              <div style={{marginTop:14}}><FlagCard flag={writer.flags[0]} /></div>
            </div>
            <div>
              <h4 className="fs-block-h"><I.check width={13} height={13} style={{verticalAlign:"-2px",marginRight:6}}/>Check against criteria</h4>
              <CriteriaList items={gate.criteria} />
            </div>
          </div>
          <div className="fs-divider"></div>
          <div style={{display:"flex",gap:12,alignItems:"center",flexWrap:"wrap"}}>
            <Btn variant="approve" size="lg" icon={<I.check width={18} height={18}/>} onClick={lc.approve}>Approve &amp; resume</Btn>
            <Btn variant="changes" size="lg" icon={<I.back width={17} height={17}/>} onClick={lc.requestChanges}>Request changes</Btn>
            <div className="ga-note" style={{marginLeft:"auto"}}>A confident handoff — approve to resume Vera's final pass, or send it back to Tomás.</div>
          </div>
        </React.Fragment>
      );
    }
    if(status==="done"){
      return (
        <div style={{display:"flex",flexDirection:"column",alignItems:"flex-start",gap:16}}>
          <span className="kava kava--lg" style={{background:"linear-gradient(155deg,#FFC56E,#E8870E)",color:"#3a1d00"}}><I.hand width={22} height={22}/></span>
          <div><div className="fs-name">Approved by you</div><div className="fs-role" style={{marginTop:8}}>You reviewed the technical volume and approved it. Vera's final compliance pass resumed automatically.</div></div>
          <span className="kbadge kbadge--done"><span className="dot"></span>Approved</span>
        </div>
      );
    }
    return <div className="fs-role" style={{fontSize:15}}>The system will pause here once Tomás finishes the draft, and hand the review to you.</div>;
  }

  if(stage.kind==="terminal"){
    if(lc.phase==="complete"){
      return (
        <div style={{display:"flex",flexDirection:"column",alignItems:"flex-start",gap:18}}>
          <span className="kava kava--lg" style={{width:60,height:60,borderRadius:17,background:"linear-gradient(155deg,#2BD49A,#15A06B)"}}><I.check width={28} height={28}/></span>
          <div><div className="fs-name">Package ready to submit</div><div className="fs-role" style={{marginTop:8,maxWidth:"54ch"}}>All stages complete · 24/24 requirements addressed · validated for CISA format. Final human submission to SAM.gov.</div></div>
          <div style={{display:"flex",gap:11}}>
            <Btn variant="ghost" onClick={lc.restart}>Replay</Btn>
            <Btn variant="select" size="lg" icon={<I.arrow width={18} height={18}/>}>Submit to SAM.gov</Btn>
          </div>
        </div>
      );
    }
    return <div className="fs-role" style={{fontSize:15}}>Once the final review clears, the package lands here, ready for your submission.</div>;
  }

  // agent stage
  const agent = window.KAIMI_AGENTS[stage.agentKey];
  return (
    <React.Fragment>
      <div className="fs-head">
        <Avatar agent={agent} size="lg" working={status==="progress"} />
        <div className="fs-id">
          <div className="fs-name">{stage.name}</div>
          <div className="fs-role"><b>{agent.name}</b> · {agent.role} agent</div>
        </div>
        <div className="fs-st"><StatusBadge status={status} /></div>
      </div>
      <div className="fs-divider"></div>
      {status==="progress" && (
        <div className="fs-live">
          <div className="fl-bar"><span className="live2"></span>{agent.name} is working · live</div>
          <div style={{color:"var(--ink-2)",fontSize:15,marginBottom:18}}>{stage.working}</div>
          <div className="fs-line" style={{width:"92%"}}></div>
          <div className="fs-line" style={{width:"86%"}}></div>
          <div className="fs-line" style={{width:"94%"}}></div>
          <div className="fs-line" style={{width:"70%"}}></div>
          <div className="fs-line" style={{width:"40%"}}></div>
        </div>
      )}
      {status==="done" && (
        <React.Fragment>
          <div className="fs-summary">{stage.summary}</div>
          <h4 className="fs-block-h">Produced</h4>
          <Metrics items={stage.metrics} />
          <Artifacts items={stage.artifacts} />
          {stage.flags.map((f,i)=><div key={i} style={{marginTop:16}}><FlagCard flag={f} /></div>)}
        </React.Fragment>
      )}
      {status==="pending" && <div className="fs-role" style={{fontSize:15}}>Waiting on the previous stage to finish before {agent.name} starts.</div>}
    </React.Fragment>
  );
}

function TakeB({ lc }){
  const stages = window.KAIMI_STAGES;
  const [viewIdx, setViewIdx] = React.useState(lc.currentIndex);
  const prevCur = React.useRef(lc.currentIndex);
  React.useEffect(()=>{
    if(lc.currentIndex !== prevCur.current){ setViewIdx(lc.currentIndex); prevCur.current = lc.currentIndex; }
  }, [lc.currentIndex]);

  const stage = stages[viewIdx];
  const status = lc.statuses[viewIdx];
  const spotlight = stage.kind==="gate" && status==="human";

  return (
    <div className="tb-body" data-spot={spotlight?"1":undefined}>
      <div className="tb-rail">
        <div className="rail-h">Pipeline · 5 stages</div>
        {stages.map((s,i)=>(
          <RailChip key={s.id} stage={s} status={lc.statuses[i]} active={viewIdx===i} onClick={()=>setViewIdx(i)} />
        ))}
      </div>
      <div className="tb-stage" data-gate={spotlight?"1":undefined}>
        <FocusContent stage={stage} status={status} lc={lc} />
      </div>
    </div>
  );
}

window.TakeB = TakeB;
