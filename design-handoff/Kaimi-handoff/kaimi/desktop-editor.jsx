/* ============================================================
   KAIMI Desktop — Working-draft editor
   Section-structured, human-editable, attribution + autosave.
   Edits are first-class: Final Review runs on what you leave here.
   ============================================================ */

const ED_SECTIONS = [
  { no:"1", title:"Technical Approach", reqs:"5 reqs", state:"ok",
    body:["Our approach implements CISA's Zero Trust Maturity Model across all five pillars, beginning with a 90-day discovery sprint that inventories identities, devices, and data flows against the current architecture.","We phase migration by mission impact: identity and device pillars first, establishing continuous verification before network segmentation begins."] },
  { no:"2", title:"Zero Trust Architecture", reqs:"6 reqs", state:"ok",
    body:["The target architecture centers on a policy decision point integrated with the agency's existing ICAM stack, enforcing per-session, least-privilege access for every request.","Microsegmentation is delivered through software-defined perimeters, with east-west traffic inspection at every enclave boundary."] },
  { no:"3", title:"Past Performance", reqs:"4 reqs", state:"warn", flagged:true,
    body:["BlueMeta has delivered zero-trust and cybersecurity engineering across two recent federal engagements: a $1.8M ICAM modernization for a civilian agency and a $1.6M cloud security baseline for a DoD component.","Both engagements achieved CPARS ratings of Exceptional and are directly relevant to the technical scope of this solicitation."] },
  { no:"4", title:"Management & Staffing", reqs:"4 reqs", state:"ok",
    body:["The program is led by a cleared PM with 12 years of federal cybersecurity delivery, supported by a dedicated zero-trust architect and two DevSecOps engineers, all CONUS-based.","Staffing reaches full strength within 30 days of award; key personnel resumes are included in Volume II."] },
  { no:"5", title:"Transition & Risk", reqs:"2 reqs", state:"ok",
    body:["Transition follows a two-week knowledge-capture period with the incumbent, with rollback checkpoints at each migration phase to guarantee continuity of operations."] },
  { no:"6", title:"Security & Compliance", reqs:"2 reqs", state:"ok",
    body:["All work is performed under the agency's ATO boundary; our delivery framework maps controls to NIST 800-207 and 800-53 rev 5, with evidence packages produced continuously rather than at audit time."] },
  { no:"7", title:"Quality Assurance", reqs:"1 req", state:"ok",
    body:["A monthly quality gate reviews deliverables against the QASP, with metrics reported through the COR dashboard."] },
];

function DraftEditor({ proposal, online, onBack }){
  const [cur, setCur] = React.useState(0);
  const [edited, setEdited] = React.useState(false);
  const [saveState, setSaveState] = React.useState("saved"); // saved | saving
  const scrollRef = React.useRef(null);
  const secRefs = React.useRef([]);
  const saveTimer = React.useRef(null);

  const onEdit = ()=>{
    setEdited(true);
    setSaveState("saving");
    clearTimeout(saveTimer.current);
    saveTimer.current = setTimeout(()=> setSaveState("saved"), 900);
  };
  React.useEffect(()=> ()=> clearTimeout(saveTimer.current), []);

  const jump = (i)=>{
    setCur(i);
    const el = secRefs.current[i], sc = scrollRef.current;
    if(el && sc) sc.scrollTop = el.offsetTop - 24;
  };

  return (
    <div className="ed">
      <div className="ed-rail">
        <div className="er-h">Sections · 7</div>
        {ED_SECTIONS.map((s,i)=>(
          <button key={i} className={`ed-sec ${s.state==="warn"?"warn":""} ${cur===i?"cur":""}`} onClick={()=>jump(i)}>
            <span className="dot"></span>
            <span style={{minWidth:0}}>
              <b>{s.no} · {s.title}</b>
              <span className="sub">{s.reqs}{s.flagged ? " · ⚑ gap" : ""}</span>
            </span>
          </button>
        ))}
      </div>

      <div className="ed-main">
        <div className="ed-top">
          <Btn variant="ghost" size="sm" icon={<I.back width={15} height={15}/>} onClick={onBack}>Back to review</Btn>
          <div className="ed-name">Technical Volume — working draft<span>{proposal ? proposal.title : ""}</span></div>
          <div style={{flex:1}}></div>
          <span className={`ed-ver ${edited?"you":""}`}>{edited ? "v4 · edited by you" : "v3 · Tomás"}</span>
          <span className={`ed-save ${saveState==="saving"?"saving":""}`}>
            <span className="sdot"></span>
            {saveState==="saving" ? "Saving…" : online ? "Saved" : "Saved to this device"}
          </span>
        </div>

        <div className="ed-scroll" ref={scrollRef}>
          <div className="ed-doc">
            <div className="ed-title">Zero Trust Architecture Modernization — Technical Volume</div>
            <div className="ed-meta">DHS · CISA · SOL# 70RCSA24R0000123 · 18 pp · compliance 22/24 · click any paragraph to edit</div>
            {ED_SECTIONS.map((s,i)=>(
              <section key={i} ref={el=>secRefs.current[i]=el}>
                <div className="sec-head2">
                  <h3><span className="no">{s.no}</span>{s.title}</h3>
                  <span className="reqtag">{s.reqs}</span>
                </div>
                <div contentEditable suppressContentEditableWarning spellCheck="false" onInput={onEdit}>
                  {s.body.map((p,j)=><p key={j}>{p}</p>)}
                </div>
                {s.flagged && (
                  <div className="ed-flag">
                    <span className="ef-ic"><I.warn width={15} height={15}/></span>
                    <div>
                      <b>Tomás flagged this section: no past-performance at this scale</b>
                      <p>Neither cited contract exceeds $2M against a $4.8M ceiling. Consider naming a teaming partner here, or strengthening the relevance argument — your edits carry into Vera's final pass.</p>
                    </div>
                  </div>
                )}
              </section>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

window.DraftEditor = DraftEditor;
