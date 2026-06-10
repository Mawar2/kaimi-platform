/* ============================================================
   KAIMI Desktop — Onboarding flow
   welcome → SSO sign-in → license key → company profile →
   meet the team → first sync → into the app
   ============================================================ */

const OB_STEPS = ["Welcome","Sign in","License","Profile","Your team","First sync"];

const OB_NAICS = [
  { code:"541512", label:"Computer Systems Design" },
  { code:"541511", label:"Custom Programming" },
  { code:"541519", label:"Other Computer Services" },
  { code:"541513", label:"Facilities Management" },
  { code:"541715", label:"R&D Physical/Engineering" },
  { code:"518210", label:"Data Processing & Hosting" },
];

function ObBrandPanel({ step }){
  const quotes = [
    "Kaimi — Hawaiian for \u201Cthe seeker.\u201D",
    "Your work account stays yours — Kaimi only asks for your name and email.",
    "One license, your whole BD pipeline.",
    "Kaimi scores every opportunity against what your company actually does.",
    "Agents draft. You decide. Always.",
    "From here on, the hunt runs every night at 02:00.",
  ];
  return (
    <div className="ob-brand">
      <div className="ob-mark">
        <svg width="40" height="40" viewBox="0 0 64 64" fill="none">
          <circle cx="45" cy="18.5" r="6.6" fill="#22D3EE"/>
          <path d="M9 37.5C16.5 28 23.5 28 31 37.5C38.5 47 45.5 47 53 37.5" stroke="#5B9BFF" strokeWidth="4.8" strokeLinecap="round"/>
          <path d="M9 47.5C16.5 38 23.5 38 31 47.5C38.5 57 45.5 57 53 47.5" stroke="#fff" strokeWidth="4.8" strokeLinecap="round" opacity="0.92"/>
        </svg>
        <div className="nm">Kaimi<small>THE SEEKER · BY BLUEMETA</small></div>
      </div>
      <svg className="waves" viewBox="0 0 600 200" preserveAspectRatio="none" fill="none">
        <path d="M-20 90C80 30 180 30 280 90C380 150 480 150 620 90" stroke="rgba(34,211,238,0.5)" strokeWidth="3"/>
        <path d="M-20 140C80 80 180 80 280 140C380 200 480 200 620 140" stroke="rgba(91,155,255,0.35)" strokeWidth="3"/>
        <path d="M-20 190C80 130 180 130 280 190C380 250 480 250 620 190" stroke="rgba(255,255,255,0.18)" strokeWidth="3"/>
      </svg>
      <h2>The agents hunt. <em>You</em> make the calls.</h2>
      <p>Kaimi finds and scores federal opportunities every night, drafts the proposals worth pursuing, and pauses for your review before anything ships.</p>
      <div className="ob-quote">{quotes[step]}</div>
    </div>
  );
}

function ObProgress({ step }){
  return (
    <div className="ob-prog">
      {OB_STEPS.map((s,i)=>(
        <div key={i} className={`pseg ${i<step?"done":i===step?"cur":""}`}></div>
      ))}
      <span className="plabel">{step+1} / {OB_STEPS.length}</span>
    </div>
  );
}

function OnboardingFlow({ onDone }){
  const [step, setStep] = React.useState(0);
  // sign in
  const [account, setAccount] = React.useState(null);
  // license
  const [key, setKey] = React.useState("");
  const [keyState, setKeyState] = React.useState("idle"); // idle | checking | ok
  // profile
  const [naics, setNaics] = React.useState(new Set(["541512","541511"]));
  const [docs, setDocs] = React.useState([
    { name:"CISA-IDIQ-2023-pastperf.pdf", size:"2.1 MB" },
    { name:"GSA-cloud-migration-CPARS.pdf", size:"1.4 MB" },
  ]);
  // sync
  const [syncStage, setSyncStage] = React.useState(0);
  const syncTimer = React.useRef([]);

  const next = ()=> setStep(s=>Math.min(s+1, OB_STEPS.length-1));

  const signIn = (provider)=>{
    setAccount({ name:"Jordan Akana", email:"jordan@bluemetatech.com", provider });
    setTimeout(next, 900);
  };

  const formatKey = (raw)=>{
    const c = raw.toUpperCase().replace(/[^A-Z0-9]/g,"").slice(0,16);
    return c.match(/.{1,4}/g)?.join("-") || "";
  };
  const validateKey = ()=>{
    setKeyState("checking");
    setTimeout(()=> setKeyState("ok"), 1400);
  };

  const toggleNaics = (code)=> setNaics(prev=>{
    const n = new Set(prev); n.has(code) ? n.delete(code) : n.add(code); return n;
  });
  const addDoc = ()=>{
    const pool = [
      { name:"VA-data-platform-pastperf.pdf", size:"3.2 MB" },
      { name:"DISA-SOC-reference.pdf", size:"890 KB" },
      { name:"capabilities-statement-2026.pdf", size:"1.1 MB" },
    ];
    setDocs(prev => prev.length-2 < pool.length ? [...prev, pool[prev.length-2]] : prev);
  };

  const startSync = ()=>{
    next();
    [1,2,3,4,5].forEach((stage,i)=>{
      syncTimer.current.push(setTimeout(()=> setSyncStage(stage), 900*(i+1)));
    });
  };
  React.useEffect(()=> ()=> syncTimer.current.forEach(clearTimeout), []);

  const SYNC_LINES = [
    { t:"Linking your license", m:"KAIMI-\u2022\u2022\u2022\u2022-7Q2F" },
    { t:"Pulling tonight's queue from SAM.gov", m:"9 opportunities" },
    { t:"Indexing past performance", m:`${docs.length} documents` },
    { t:"Waking your agent team", m:"Noa · Tomás · Vera" },
    { t:"Preparing the offline cache", m:"drafts work anywhere" },
  ];

  return (
    <div className="ob">
      <ObBrandPanel step={step} />
      <div className="ob-step">
        <div className="ob-step-inner">
          <ObProgress step={step} />

          {step===0 && (
            <React.Fragment>
              <div className="ob-h1">Welcome to Kaimi</div>
              <p className="ob-lead">Set up takes about three minutes: sign in with your work account, link your license, and tell Kaimi what your company does — so tonight's hunt is already yours.</p>
              <div className="ob-body">
                <div className="team">
                  <div className="tcard">
                    <span className="kava kava--lg" style={{background:"linear-gradient(155deg,#5B9BFF,#2563EB)"}}>N</span>
                    <div><b>It hunts while you sleep</b><p>Every night Kaimi pulls live SAM.gov opportunities and scores each one against your capabilities.</p></div>
                  </div>
                  <div className="tcard">
                    <span className="kava kava--lg" style={{background:"linear-gradient(155deg,#67E0F4,#0EA5C4)", color:"#062a33"}}>T</span>
                    <div><b>It drafts the ones you pick</b><p>Select an opportunity and a team of agents outlines, writes, and checks the proposal.</p></div>
                  </div>
                  <div className="tcard">
                    <span className="kava kava--lg" style={{background:"linear-gradient(155deg,#FFC56E,#E8870E)"}}><I.hand width={20} height={20}/></span>
                    <div><b>You stay in command</b><p>Nothing ships without you. Every proposal pauses at one human review gate — yours.</p></div>
                  </div>
                </div>
              </div>
              <div className="ob-foot">
                <Btn variant="primary" size="lg" icon={<I.arrow width={17} height={17}/>} onClick={next}>Get started</Btn>
                <div className="note">Works offline once set up — drafts and reviews save to this device.</div>
              </div>
            </React.Fragment>
          )}

          {step===1 && (
            <React.Fragment>
              <div className="ob-h1">Sign in with your work account</div>
              <p className="ob-lead">Kaimi uses your organization's identity provider — no new password to manage.</p>
              <div className="ob-body">
                {!account ? (
                  <div className="sso">
                    <button onClick={()=>signIn("Google")}>
                      <span className="glyph" style={{color:"#4285F4"}}>G</span>Continue with Google Workspace
                    </button>
                    <button onClick={()=>signIn("Microsoft")}>
                      <span className="glyph"><svg width="18" height="18" viewBox="0 0 18 18"><rect width="8" height="8" fill="#F25022"/><rect x="10" width="8" height="8" fill="#7FBA00"/><rect y="10" width="8" height="8" fill="#00A4EF"/><rect x="10" y="10" width="8" height="8" fill="#FFB900"/></svg></span>Continue with Microsoft Entra
                    </button>
                  </div>
                ) : (
                  <div className="signed">
                    <span className="av">JA</span>
                    <div><b>{account.name}</b><span>{account.email} · via {account.provider}</span></div>
                    <span className="ok"><I.check width={20} height={20}/></span>
                  </div>
                )}
              </div>
              <div className="ob-foot">
                {account && <Btn variant="primary" size="lg" icon={<I.arrow width={17} height={17}/>} onClick={next}>Continue</Btn>}
                <div className="note">Your admin can manage seats from the BlueMeta console.</div>
              </div>
            </React.Fragment>
          )}

          {step===2 && (
            <React.Fragment>
              <div className="ob-h1">Link your Kaimi license</div>
              <p className="ob-lead">Your license key connects this device to your team's subscription and the agent runtime.</p>
              <div className="ob-body">
                {keyState!=="ok" ? (
                  <React.Fragment>
                    <div className="keyrow">
                      <input className="keyinput" placeholder="KAIMI-XXXX-XXXX-XXXX" value={key}
                        onChange={e=>setKey(formatKey(e.target.value))} spellCheck="false" />
                      <Btn variant="primary" size="lg" onClick={validateKey} disabled={key.replace(/-/g,"").length<12 || keyState==="checking"}>
                        {keyState==="checking" ? "Checking…" : "Validate"}
                      </Btn>
                    </div>
                    <div className="keyhint">Find it in your welcome email, or in the BlueMeta console under <a href="#" onClick={e=>e.preventDefault()}>Licenses</a>. Keys look like KAIMI-9X2B-44KD-7Q2F.</div>
                  </React.Fragment>
                ) : (
                  <div className="keycard">
                    <span className="kc-ic"><I.check width={19} height={19}/></span>
                    <div>
                      <b>License verified</b>
                      <div className="kc-meta">BlueMeta Technologies · Team plan · 5 seats · key <code>····{key.slice(-4) || "7Q2F"}</code></div>
                    </div>
                  </div>
                )}
              </div>
              <div className="ob-foot">
                {keyState==="ok" && <Btn variant="primary" size="lg" icon={<I.arrow width={17} height={17}/>} onClick={next}>Continue</Btn>}
                <div className="note">One key per organization — seats are managed per sign-in.</div>
              </div>
            </React.Fragment>
          )}

          {step===3 && (
            <React.Fragment>
              <div className="ob-h1">What does your company do?</div>
              <p className="ob-lead">Kaimi scores every opportunity against this profile. The better it knows you, the sharper the fit scores.</p>
              <div className="ob-body">
                <div className="ob-sec-h">NAICS codes — pick all that apply</div>
                <div className="naics-wrap">
                  {OB_NAICS.map(n=>(
                    <button key={n.code} className={`naics ${naics.has(n.code)?"on":""}`} onClick={()=>toggleNaics(n.code)}>
                      {n.code}<small>{n.label}</small>
                    </button>
                  ))}
                </div>
                <div className="ob-sec-h">Capabilities statement</div>
                <textarea className="caparea" defaultValue="Cybersecurity engineering and zero-trust architecture for federal agencies; cloud migration and DevSecOps; enterprise data platforms. CONUS, cleared staff, ISO 27001."></textarea>
                <div className="ob-sec-h">Past performance — used as evidence in drafts</div>
                {docs.map((d,i)=>(
                  <div className="uprow" key={i}><I.doc/>{d.name}<span className="sz">{d.size}</span></div>
                ))}
                <div className="updrop" onClick={addDoc} role="button" tabIndex={0}
                  onKeyDown={e=>{if(e.key==="Enter")addDoc();}}>
                  <span className="ic"><svg width="17" height="17" viewBox="0 0 24 24" fill="none"><path d="M12 16V4M7 9l5-5 5 5M5 20h14" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/></svg></span>
                  Drop CPARS, references, or past proposals here — or click to browse
                </div>
              </div>
              <div className="ob-foot">
                <Btn variant="primary" size="lg" icon={<I.arrow width={17} height={17}/>} onClick={next} disabled={naics.size===0}>Continue</Btn>
                <button className="skip" onClick={next}>Finish this later</button>
                <div className="note">You can refine the profile any time in Settings.</div>
              </div>
            </React.Fragment>
          )}

          {step===4 && (
            <React.Fragment>
              <div className="ob-h1">Meet your agent team</div>
              <p className="ob-lead">Three specialists work every proposal you select. They report progress as they go — and they know when to stop and ask you.</p>
              <div className="ob-body">
                <div className="team">
                  <div className="tcard">
                    <Avatar agent={window.KAIMI_AGENTS.outline} size="lg" />
                    <div><b>Noa</b><div className="role">Outline</div><p>Reads the solicitation and maps it into a section plan — every requirement accounted for before a word is written.</p></div>
                  </div>
                  <div className="tcard">
                    <Avatar agent={window.KAIMI_AGENTS.writer} size="lg" />
                    <div><b>Tomás</b><div className="role">Technical Writer</div><p>Drafts the technical volume section by section, in the agency's required format, and flags any gaps he finds.</p></div>
                  </div>
                  <div className="tcard">
                    <Avatar agent={window.KAIMI_AGENTS.review} size="lg" />
                    <div><b>Vera</b><div className="role">Final Review</div><p>Runs the final compliance pass — requirements, formatting, cross-references — before anything is called ready.</p></div>
                  </div>
                  <div className="gate-note">
                    <span className="gn-ic"><I.hand width={17} height={17}/></span>
                    <div><b>They pause for you at one gate.</b><p>Between Tomás and Vera, every proposal stops for your review. You read, edit the draft directly, and approve — or send it back. You always make the call.</p></div>
                  </div>
                </div>
              </div>
              <div className="ob-foot">
                <Btn variant="primary" size="lg" icon={<I.arrow width={17} height={17}/>} onClick={startSync}>Run my first sync</Btn>
              </div>
            </React.Fragment>
          )}

          {step===5 && (
            <React.Fragment>
              <div className="ob-h1">{syncStage<5 ? "Setting up your pipeline" : "You're set."}</div>
              <p className="ob-lead">{syncStage<5 ? "Pulling tonight's hunt and preparing this device to work anywhere — even offline." : "Tonight's queue is in, your team is awake, and this device can work offline. The hunt runs nightly at 02:00."}</p>
              <div className="ob-body">
                <div className="sync">
                  {SYNC_LINES.map((l,i)=>(
                    <div key={i} className={`syncline ${syncStage>i ? "ok" : syncStage===i ? "run" : ""}`}>
                      <span className="si">{syncStage>i ? <I.check/> : syncStage===i ? <I.spinner/> : <I.dot style={{opacity:.4}}/>}</span>
                      {l.t}<span className="meta">{l.m}</span>
                    </div>
                  ))}
                  {syncStage>=5 && (
                    <div className="sync-done">
                      <span className="sd-ic"><I.check width={22} height={22}/></span>
                      <div><b>9 opportunities are waiting in your queue</b><span>Top fit tonight: 82 — Zero Trust Architecture Modernization (DHS · CISA).</span></div>
                    </div>
                  )}
                </div>
              </div>
              <div className="ob-foot">
                {syncStage>=5 && <Btn variant="select" size="lg" icon={<I.arrow width={18} height={18}/>} onClick={onDone}>Enter Kaimi</Btn>}
              </div>
            </React.Fragment>
          )}

        </div>
      </div>
    </div>
  );
}

window.OnboardingFlow = OnboardingFlow;
