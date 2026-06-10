/* ============================================================
   KAIMI — Hero · TAKE A — "Timeline Rail"
   Horizontal pipeline stepper + a vertical stack of stage cards.
   The most legible take. The gate becomes a prominent inline panel.
   ============================================================ */

function StageCardA({ stage, status, isLast }){
  const agent = window.KAIMI_AGENTS[stage.agentKey];
  return (
    <div className="stage-card" data-st={status}>
      <div className="sc-rail">
        <Avatar agent={agent} working={status==="progress"} />
        {!isLast && <div className="conn"></div>}
      </div>
      <div className="sc-main">
        <div className="sc-top">
          <div className="sc-id">
            <div className="sc-name">{stage.name}</div>
            <div className="sc-role"><b>{agent.name}</b> · {agent.role} agent</div>
          </div>
          <div className="sc-status"><StatusBadge status={status} /></div>
        </div>
        {status==="progress" && (
          <div className="sc-working"><span className="typ"><i></i><i></i><i></i></span>{stage.working}</div>
        )}
        {status==="done" && (
          <React.Fragment>
            <div className="sc-summary">{stage.summary}</div>
            <Metrics items={stage.metrics} />
            <Artifacts items={stage.artifacts} />
            {stage.flags.map((f,i)=><FlagCard key={i} flag={f} />)}
          </React.Fragment>
        )}
        {status==="pending" && <div className="sc-summary" style={{opacity:.75}}>Waiting on the previous stage.</div>}
      </div>
    </div>
  );
}

function GatePanelA({ lc }){
  const gate = window.KAIMI_STAGES.find(s=>s.id==="gate");
  const writer = window.KAIMI_STAGES.find(s=>s.id==="writer");
  return (
    <div className="gate-panel">
      <div className="gate-glow"></div>
      <div className="gate-head">
        <div className="gate-badge"><I.hand width={14} height={14}/>Needs you</div>
        <div className="gh-text">
          <h2>Tomás is handing you the draft</h2>
          <p>{gate.prompt}</p>
        </div>
        <GateHandoff agent={window.KAIMI_AGENTS.writer} />
      </div>
      <div className="gate-body">
        <div className="gate-col">
          <h4><I.doc width={14} height={14}/> The draft to review</h4>
          <DraftPreview />
          <FlagCard flag={writer.flags[0]} />
        </div>
        <div className="gate-col">
          <h4><I.check width={14} height={14}/> Check against criteria</h4>
          <CriteriaList items={gate.criteria} />
        </div>
      </div>
      <GateActions onApprove={lc.approve} onChanges={lc.requestChanges} />
    </div>
  );
}

function GateApprovedA(){
  return (
    <div className="stage-card" data-st="done">
      <div className="sc-rail">
        <span className="kava" style={{background:"linear-gradient(155deg,#FFC56E,#E8870E)", color:"#3a1d00"}}><I.hand width={18} height={18}/></span>
        <div className="conn"></div>
      </div>
      <div className="sc-main">
        <div className="sc-top">
          <div className="sc-id">
            <div className="sc-name">Human Review</div>
            <div className="sc-role"><b>You</b> · approved the draft</div>
          </div>
          <span className="kbadge kbadge--done"><span className="dot"></span>Approved</span>
        </div>
        <div className="sc-summary">You reviewed the technical volume and approved it. Vera's final compliance pass resumed automatically.</div>
      </div>
    </div>
  );
}

function GatePendingA(){
  return (
    <div className="stage-card" data-st="pending">
      <div className="sc-rail">
        <span className="kava" style={{background:"var(--surface-3)", color:"var(--ink-3)"}}><I.hand width={18} height={18}/></span>
        <div className="conn"></div>
      </div>
      <div className="sc-main">
        <div className="sc-top">
          <div className="sc-id"><div className="sc-name">Human Review</div><div className="sc-role">Waiting on the technical volume</div></div>
          <StatusBadge status="pending" label="Upcoming" />
        </div>
        <div className="sc-summary" style={{opacity:.75}}>When the draft is ready, the system pauses here for your review.</div>
      </div>
    </div>
  );
}

function TakeA({ lc }){
  const stages = window.KAIMI_STAGES;
  const gateIdx = stages.findIndex(s=>s.id==="gate");
  const gateStatus = lc.statuses[gateIdx];

  return (
    <div className="ta-body">
      <PipelineStepper statuses={lc.statuses} currentIndex={lc.currentIndex} />
      <div className="ta-stack">
        {stages.map((s, i) => {
          if(s.kind==="agent"){
            return <StageCardA key={s.id} stage={s} status={lc.statuses[i]} isLast={false} />;
          }
          if(s.kind==="gate"){
            if(gateStatus==="human") return <div className="stage-card gate-card" key={s.id}><GatePanelA lc={lc} /></div>;
            if(gateStatus==="done")  return <GateApprovedA key={s.id} />;
            return <GatePendingA key={s.id} />;
          }
          // terminal
          if(lc.phase==="complete"){
            return <DoneBanner key={s.id} onRestart={lc.restart} />;
          }
          return (
            <div className="stage-card" data-st="pending" key={s.id}>
              <div className="sc-rail"><span className="kava" style={{background:"var(--surface-3)",color:"var(--ink-3)"}}><I.arrow width={18} height={18}/></span></div>
              <div className="sc-main"><div className="sc-top"><div className="sc-id"><div className="sc-name">Ready to Submit</div><div className="sc-role">Final human submission to SAM.gov</div></div><StatusBadge status="pending" label="Upcoming" /></div></div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

window.TakeA = TakeA;
