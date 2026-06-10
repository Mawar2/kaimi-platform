/* ============================================================
   KAIMI — Hero · TAKE C — "Split Command"
   Left: vertical pipeline + live agent activity feed.
   Right: the artifact canvas (the document being built) with an
   approval dock that rises when the gate needs you.
   ============================================================ */

function VStage({ stage, status, isLast }){
  const agent = stage.agentKey ? window.KAIMI_AGENTS[stage.agentKey] : null;
  let icon;
  if(stage.kind==="gate") icon = <I.hand/>;
  else if(stage.kind==="terminal") icon = status==="done" ? <I.check/> : <I.arrow/>;
  else if(status==="done") icon = <I.check/>;
  else if(status==="progress") icon = <I.spinner/>;
  else icon = <I.dot style={{opacity:.5}}/>;
  const meta = stage.kind==="agent"
    ? <span><b style={{color:"var(--ink-2)",fontWeight:600}}>{agent.name}</b> · {status==="progress"?"working":status==="done"?"done":"pending"}</span>
    : stage.kind==="gate" ? "Your review gate" : "Final submission";
  return (
    <div className="vstage" data-st={status}>
      <div className="vrail">
        <div className="vring">{icon}</div>
        {!isLast && <div className="vline"></div>}
      </div>
      <div className="vinfo">
        <div className="vname">{stage.name}</div>
        <div className="vmeta">{meta}</div>
      </div>
      {status==="human" && <span className="vbadge"><StatusBadge status="human" /></span>}
      {status==="progress" && <span className="vbadge"><StatusBadge status="progress" /></span>}
    </div>
  );
}

function FeedItem({ entry }){
  const ic = {
    start:<I.spinner/>, done:<I.check/>, flag:<I.flag/>, human:<I.hand/>,
    approve:<I.check/>, changes:<I.back/>, ready:<I.arrow/>,
  }[entry.kind] || <I.dot/>;
  return (
    <div className={`feed-item k-${entry.kind}`}>
      <span className="fi-time">{entry.time}</span>
      <span className="fi-ic">{ic}</span>
      <span className="fi-text">{entry.text}</span>
    </div>
  );
}

function CanvasDoc({ lc }){
  const flagged = lc.atGate || lc.statuses[lc.stages.findIndex(s=>s.id==="gate")]==="human";
  const complete = lc.phase==="complete";
  return (
    <div className="cv-doc">
      <h3>Technical Volume — Zero Trust Architecture Modernization</h3>
      <div className="cv-meta">{complete ? "technical-volume-FINAL.pdf · 20 pp" : "technical-volume-draft-v3.docx · 18 pp"} · CISA · SOL# 70RCSA24R0000123</div>

      <div className="cv-sec">
        <div className="cv-sh">1 · Technical Approach</div>
        <div className="cv-line" style={{width:"96%"}}></div>
        <div className="cv-line" style={{width:"90%"}}></div>
        <div className="cv-line" style={{width:"40%"}}></div>
      </div>
      <div className="cv-sec">
        <div className="cv-sh">2 · Zero Trust Architecture</div>
        <div className="cv-line" style={{width:"94%"}}></div>
        <div className="cv-line" style={{width:"88%"}}></div>
        <div className="cv-line" style={{width:"55%"}}></div>
      </div>
      <div className="cv-sec" style={ flagged ? {padding:"14px 16px",borderRadius:"var(--r-md)",background:"var(--st-human-bg)",border:"1px solid color-mix(in oklab,var(--st-human) 36%,transparent)",margin:"20px -16px 0"} : null}>
        <div className="cv-sh" style={ flagged ? {color:"var(--st-human)"} : null}>
          3 · Past Performance {flagged && !complete && <span style={{fontSize:11,marginLeft:8,textTransform:"none",letterSpacing:0}}>⚑ gap flagged</span>}
          {complete && <span style={{fontSize:11,marginLeft:8,textTransform:"none",letterSpacing:0,color:"var(--st-done)"}}>✓ resolved</span>}
        </div>
        <div className="cv-line" style={{width:"92%"}}></div>
        <div className="cv-line" style={{width:complete?"86%":"48%"}}></div>
        {flagged && !complete && <div style={{fontSize:12.5,color:"var(--st-human-tint)",marginTop:8,lineHeight:1.5}}>No past-performance for cybersecurity at this scale — recommend a teaming partner.</div>}
      </div>
      <div className="cv-sec">
        <div className="cv-sh">4 · Management &amp; Staffing</div>
        <div className="cv-line" style={{width:"90%"}}></div>
        <div className="cv-line" style={{width:"62%"}}></div>
      </div>
    </div>
  );
}

function TakeC({ lc }){
  const stages = window.KAIMI_STAGES;
  const feedRef = React.useRef();
  React.useEffect(()=>{ if(feedRef.current) feedRef.current.scrollTop = feedRef.current.scrollHeight; }, [lc.log.length]);
  const gate = stages.find(s=>s.id==="gate");
  const writerDone = lc.statuses[stages.findIndex(s=>s.id==="writer")]==="done";
  const complete = lc.phase==="complete";

  const cvStatus = complete ? <span className="kbadge kbadge--done"><span className="dot"></span>Submission-ready</span>
    : lc.atGate ? <StatusBadge status="human" label="Awaiting your review" />
    : lc.phase==="resuming" ? <StatusBadge status="progress" label="Final pass" />
    : <StatusBadge status="progress" label="Drafting" />;

  return (
    <div className="tc-body" style={{height:"calc(100vh - 210px)", minHeight:560}}>
      {/* LEFT — pipeline + feed */}
      <div className="tc-col">
        <div className="tc-vpipe">
          {stages.map((s,i)=>(
            <VStage key={s.id} stage={s} status={lc.statuses[i]} isLast={i===stages.length-1} />
          ))}
        </div>
        <div className="tc-feed">
          <div className="feed-h"><span className="live3"></span>Agent activity</div>
          <div className="feed-scroll" ref={feedRef}>
            {lc.log.map((e,i)=><FeedItem key={i} entry={e} />)}
          </div>
        </div>
      </div>

      {/* RIGHT — artifact canvas + approval dock */}
      <div className="tc-canvas">
        <div className="cv-bar">
          <div>
            <div className="cv-title">{complete ? "Final package" : "Working draft"}</div>
            <div className="cv-sub">{complete ? "Validated for CISA format · 24/24 requirements" : "Built live by the agent pipeline"}</div>
          </div>
          <div className="cv-st">{cvStatus}</div>
        </div>
        <div className="cv-scroll">
          {writerDone ? <CanvasDoc lc={lc} /> : (
            <div className="cv-empty">
              <div className="ce-g"><I.doc width={26} height={26}/></div>
              <div style={{maxWidth:"34ch"}}>The technical volume is being assembled. Noa mapped the outline; Tomás is drafting it now.</div>
            </div>
          )}
        </div>
        {lc.atGate && (
          <div className="approval-dock">
            <div className="ad-top">
              <div className="gate-badge"><I.hand width={14} height={14}/>Needs you</div>
              <div className="ad-txt">Tomás handed you the draft — approve to resume, or send it back. <span>1 gap flagged.</span></div>
            </div>
            <div className="ad-crit">
              {gate.criteria.map((c,i)=>(
                <span className={`ad-cpill ${c.state}`} key={i}>{c.state==="ok"?<I.check/>:<I.warn/>}{c.label}</span>
              ))}
            </div>
            <div className="ad-actions">
              <Btn variant="approve" size="lg" icon={<I.check width={18} height={18}/>} onClick={lc.approve}>Approve &amp; resume</Btn>
              <Btn variant="changes" size="lg" icon={<I.back width={17} height={17}/>} onClick={lc.requestChanges}>Request changes</Btn>
            </div>
          </div>
        )}
        {complete && (
          <div className="approval-dock" style={{borderTopColor:"color-mix(in oklab,var(--st-done) 40%,transparent)", background:"radial-gradient(500px 160px at 18% -40%, rgba(21,160,107,0.16), transparent 70%), var(--surface-2)"}}>
            <div className="ad-top">
              <div className="gate-badge" style={{background:"linear-gradient(180deg,#2BD49A,#15A06B)",animation:"none",boxShadow:"0 6px 18px color-mix(in oklab,var(--st-done) 45%,transparent)"}}><I.check width={14} height={14}/>Ready</div>
              <div className="ad-txt">Vera cleared all 24 requirements. <span>Package is submission-ready.</span></div>
            </div>
            <div className="ad-actions">
              <Btn variant="select" size="lg" icon={<I.arrow width={18} height={18}/>}>Submit to SAM.gov</Btn>
              <Btn variant="ghost" onClick={lc.restart}>Replay</Btn>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

window.TakeC = TakeC;
