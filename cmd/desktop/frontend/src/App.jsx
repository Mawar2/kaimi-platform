/* ============================================================
   KAIMI Desktop — shell: title bar, onboarding gate, offline
   simulation, app + draft editor. Ported from the design handoff
   (Kaimi Desktop.html), wired to the Go backend for opportunities.
   ============================================================ */
import React, { useState, useEffect, useRef } from 'react';
import { Sidebar, OpportunitiesScreen, OppDrawer } from './screens.jsx';
import { ProposalsScreen } from './proposals.jsx';
import { WorkspaceScreen } from './workspace.jsx';
import { OnboardingFlow } from './onboarding.jsx';
import { DraftEditor } from './editor.jsx';
import { KAIMI_PROPOSALS, KAIMI_OPPS } from './data.js';
import { getOpportunities } from './api.js';

function TitleBar({ online, onToggleNet, queuedCount, onboarded, onReplayOnboarding }){
  return (
    <div className="titlebar">
      <div className="tb-brand">
        <svg width="22" height="22" viewBox="0 0 64 64" fill="none">
          <circle cx="45" cy="19" r="7" fill="#22D3EE"/>
          <path d="M9 38C17 28 24 28 31 38C38 48 45 48 53 38" stroke="#67E0F4" strokeWidth="5.4" strokeLinecap="round"/>
          <path d="M9 48C17 38 24 38 31 48C38 58 45 58 53 48" stroke="#fff" strokeWidth="5.4" strokeLinecap="round" opacity="0.9"/>
        </svg>
        <span className="nm">Kaimi<small>Desktop</small></span>
      </div>
      <div className="tb-drag"></div>
      {onboarded && (
        <button className={`netpill ${online?"":"off"}`} onClick={onToggleNet}
          title="Prototype control — simulates losing your connection">
          <span className="nd"></span>
          {online ? "Online · synced just now" : "Offline · working locally"}
          {!online && queuedCount>0 && <span className="q">{queuedCount} queued</span>}
        </button>
      )}
      {onboarded && (
        <button className="tb-iconbtn" onClick={onReplayOnboarding} title="Replay onboarding (prototype)">
          <svg viewBox="0 0 24 24" fill="none"><path d="M4 10a8 8 0 1 1 2 6M4 10V4m0 6h6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/></svg>
        </button>
      )}
      <div className="winctl">
        <button title="Minimize"><svg viewBox="0 0 12 12"><path d="M2 6h8" stroke="currentColor" strokeWidth="1.4"/></svg></button>
        <button title="Maximize"><svg viewBox="0 0 12 12" fill="none"><rect x="2.5" y="2.5" width="7" height="7" rx="1" stroke="currentColor" strokeWidth="1.3"/></svg></button>
        <button className="close" title="Close"><svg viewBox="0 0 12 12"><path d="M3 3l6 6M9 3l-6 6" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round"/></svg></button>
      </div>
    </div>
  );
}

export default function DesktopApp(){
  const [onboarded, setOnboarded] = useState(()=> localStorage.getItem("kaimi_dt_onboarded")==="1");
  const [online, setOnline] = useState(true);
  const onlineRef = useRef(true);
  const [route, setRoute] = useState("opps");
  const [proposals, setProposals] = useState(KAIMI_PROPOSALS);
  const [opps, setOpps] = useState(KAIMI_OPPS);
  const [filter, setFilter] = useState("all");
  const [drawerOpp, setDrawerOpp] = useState(null);
  const [pursued, setPursued] = useState(()=> new Set());
  const [workspaceId, setWorkspaceId] = useState(null);

  useEffect(()=>{ onlineRef.current = online; }, [online]);
  useEffect(()=>{ localStorage.setItem("kaimi_dt_onboarded", onboarded ? "1" : "0"); }, [onboarded]);

  // Load opportunities from the local store via the Go backend; falls back
  // to the bundled demo queue when the store is empty or bindings are absent.
  useEffect(()=>{
    let alive = true;
    getOpportunities().then(list => { if(alive && list && list.length) setOpps(list); });
    return ()=>{ alive = false; };
  }, []);

  const needsCount = proposals.filter(p=>p.status==="human").length;
  const queuedCount = proposals.filter(p=>p.status==="queued").length;

  /* ---- network toggle: reconnecting processes the queue ---- */
  const toggleNet = ()=>{
    const goingOnline = !online;
    setOnline(goingOnline);
    if(goingOnline){
      setProposals(prev => prev.map(x=>{
        if(x.status!=="queued") return x;
        return x._queued==="changes"
          ? {...x, status:"progress", stageIndex:1, agents:2, when:"Tomás revising", _queued:undefined}
          : {...x, status:"progress", stageIndex:3, agents:1, when:"Vera finalizing", _queued:undefined};
      }));
      // queued work completes shortly after reconnect
      setTimeout(()=> setProposals(prev => prev.map(x=>{
        if(x.when==="Vera finalizing" && x.stageIndex===3) return {...x, stageIndex:4, status:"done", agents:0, when:"Ready to submit"};
        if(x.when==="Tomás revising" && x.stageIndex===1) return {...x, stageIndex:2, status:"human", agents:0, when:"Paused just now"};
        return x;
      })), 2800);
    }
  };

  /* ---- actions (offline-aware) ---- */
  const openOpp = (o)=> setDrawerOpp(o);
  const selectOpp = (o)=>{
    if(!online){ return; } // pursuing requires the agent runtime
    const id = "np-"+o.id;
    setProposals(prev => prev.some(x=>x.id===id) ? prev : [{
      id, title:o.title, agency:o.agency, fit:o.fit,
      deadlineLabel:o.deadlineLabel, deadlineLevel:o.deadlineLevel,
      stageIndex:0, status:"progress", agents:1, when:"Noa outlining now"
    }, ...prev]);
    setPursued(prev => { const n=new Set(prev); n.add(o.id); return n; });
    setDrawerOpp(null);
    setRoute("proposals");
  };
  const openProposal = (p)=>{ setWorkspaceId(p.id); setRoute("workspace"); };
  const openDraft = (p)=>{ setWorkspaceId(p.id); setRoute("editor"); };

  const approve = (p)=>{
    if(!onlineRef.current){
      setProposals(prev => prev.map(x=> x.id===p.id ? {...x, status:"queued", _queued:"approve", when:"Approved offline · just now"} : x));
      return;
    }
    setProposals(prev => prev.map(x=> x.id===p.id ? {...x, stageIndex:3, status:"progress", agents:1, when:"Vera finalizing"} : x));
    setTimeout(()=> setProposals(prev => prev.map(x=> x.id===p.id && x.stageIndex===3 ? {...x, stageIndex:4, status:"done", agents:0, when:"Ready to submit"} : x)), 2600);
  };
  const requestChanges = (p)=>{
    if(!onlineRef.current){
      setProposals(prev => prev.map(x=> x.id===p.id ? {...x, status:"queued", _queued:"changes", when:"Changes noted offline · just now"} : x));
      return;
    }
    setProposals(prev => prev.map(x=> x.id===p.id ? {...x, stageIndex:1, status:"progress", agents:2, when:"Tomás revising"} : x));
    setTimeout(()=> setProposals(prev => prev.map(x=> x.id===p.id && x.stageIndex===1 ? {...x, stageIndex:2, status:"human", agents:0, when:"Paused just now"} : x)), 2600);
  };
  const submit = (p)=>{
    setProposals(prev => prev.map(x=> x.id===p.id ? {...x, status:"submitted", agents:0, when:"Submitted just now"} : x));
  };

  /* ---- ambient autonomy (server events; only while online) ---- */
  useEffect(()=>{
    if(!onboarded) return;
    const t1 = setTimeout(()=>{
      if(!onlineRef.current) return;
      setProposals(prev => prev.map(x=> (x.id==="p3" && x.status==="progress" && x.stageIndex===1)
        ? {...x, stageIndex:2, status:"human", agents:0, when:"Paused just now"} : x));
    }, 16000);
    const t2 = setTimeout(()=>{
      if(!onlineRef.current) return;
      setProposals(prev => prev.map(x=> (x.id==="p5" && x.status==="progress" && x.stageIndex===0)
        ? {...x, stageIndex:1, agents:2, when:"Tomás drafting now"} : x));
    }, 30000);
    return ()=>{ clearTimeout(t1); clearTimeout(t2); };
  }, [onboarded]);

  const wsProp = proposals.find(p=>p.id===workspaceId) || null;
  let effRoute = route;
  if((route==="workspace" || route==="editor") && !wsProp) effRoute = "proposals";

  const replayOnboarding = ()=>{ setOnboarded(false); setRoute("opps"); };

  return (
    <div className="win">
      <TitleBar online={online} onToggleNet={toggleNet} queuedCount={queuedCount}
        onboarded={onboarded} onReplayOnboarding={replayOnboarding} />

      {!onboarded ? (
        <div className="win-main">
          <OnboardingFlow onDone={()=>setOnboarded(true)} />
        </div>
      ) : (
        <React.Fragment>
          {!online && (
            <div className="offline-bar">
              <svg viewBox="0 0 24 24" fill="none"><path d="M2 8.5C7.5 4 16.5 4 22 8.5M5 12.5c4-3.5 10-3.5 14 0M8.5 16.5c2-1.8 5-1.8 7 0" stroke="currentColor" strokeWidth="2" strokeLinecap="round" opacity="0.35"/><path d="M3 3l18 18" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/><circle cx="12" cy="19.5" r="1.6" fill="currentColor"/></svg>
              <span><b>Working offline.</b> Drafts and reviews save to this device and sync when you reconnect — agent runs and the nightly hunt resume online.</span>
            </div>
          )}
          <div className="win-main">
            {effRoute==="editor" ? (
              <DraftEditor proposal={wsProp} online={online} onBack={()=>setRoute("workspace")} />
            ) : (
              <div className="app">
                <Sidebar route={effRoute} go={setRoute} needsCount={needsCount}
                  queueCount={opps.length} activeCount={proposals.filter(p=>p.status!=="submitted").length} />
                <main className="main">
                  <div className="route-fade" key={effRoute + (online?"":"-off")}>
                    {effRoute==="opps" && (
                      <OpportunitiesScreen opps={opps} onOpen={openOpp} filter={filter} setFilter={setFilter} />
                    )}
                    {effRoute==="proposals" && (
                      <ProposalsScreen proposals={proposals} onOpen={openProposal} offline={!online} />
                    )}
                    {effRoute==="workspace" && (
                      <WorkspaceScreen p={wsProp} onBack={()=>setRoute("proposals")}
                        onApprove={approve} onChanges={requestChanges} onSubmit={submit} onOpenDraft={openDraft} />
                    )}
                  </div>
                </main>
              </div>
            )}
          </div>
          {drawerOpp && (
            <OppDrawer opp={drawerOpp} onClose={()=>setDrawerOpp(null)}
              onSelect={selectOpp} pursued={pursued.has(drawerOpp.id)} offline={!online} />
          )}
        </React.Fragment>
      )}
    </div>
  );
}
