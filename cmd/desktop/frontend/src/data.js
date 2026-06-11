/* ============================================================
   KAIMI — App data · opportunity queue + active proposals
   ============================================================ */
export const KAIMI_STAGE_NAMES = ["Outline","Technical Writer","Human Review","Final Review","Submit"];

export const KAIMI_AGENTS = {
  outline: { name:"Noa",   role:"Outline",         initial:"N", hue:"blue"   },
  writer:  { name:"Tomás", role:"Technical Writer", initial:"T", hue:"cyan"   },
  review:  { name:"Vera",  role:"Final Review",     initial:"V", hue:"violet" },
};

/* The SAM.gov opportunity queue (Triage).
   Opportunities already pursued (an active proposal exists with this oppId)
   are filtered OUT of the queue by the app shell. */
export const KAIMI_OPPS = [
  { id:"o1", title:"Zero Trust Architecture Modernization", agency:"DHS · CISA", naics:"541512", sol:"70RCSA24R0000123", fit:82, rec:"bid",    deadlineLabel:"9 days",  deadlineLevel:"near", isNew:true,  day:"today", value:"$4.8M ceiling", valueNum:4.8 },
  { id:"o2", title:"Cloud Migration & DevSecOps Support",   agency:"GSA · FAS",  naics:"541512", sol:"47QTCA24R0041",   fit:76, rec:"bid",    deadlineLabel:"22 days", deadlineLevel:"soon", isNew:true,  day:"today", value:"$2.1M est.", valueNum:2.1 },
  { id:"o3", title:"Enterprise Data Platform Engineering",  agency:"VA · OIT",   naics:"541511", sol:"36C10B24R0007",   fit:71, rec:"bid",    deadlineLabel:"16 days", deadlineLevel:"soon", isNew:true,  day:"today", value:"$3.4M ceiling", valueNum:3.4 },
  { id:"o4", title:"SOC Modernization & Threat Hunting",    agency:"DoD · DISA", naics:"541519", sol:"HC102824R0019",   fit:64, rec:"review", deadlineLabel:"6 days",  deadlineLevel:"crit", isNew:true,  day:"today", value:"$6.2M ceiling", valueNum:6.2 },
  { id:"o5", title:"Identity & Access Management Services",  agency:"Treasury",   naics:"541512", sol:"2032H824R00012",  fit:58, rec:"review", deadlineLabel:"28 days", deadlineLevel:"soon", isNew:true,  day:"today", value:"$1.9M est.", valueNum:1.9 },
  { id:"o6", title:"Help Desk & End-User IT Support",       agency:"USDA",       naics:"541513", sol:"12314824R0033",   fit:34, rec:"nobid",  deadlineLabel:"11 days", deadlineLevel:"near", isNew:true,  day:"today", value:"$850K est.", valueNum:0.85 },
  { id:"o10", title:"FedRAMP Continuous Monitoring Platform", agency:"GSA · TTS", naics:"541512", sol:"47QFCA26R0019",  fit:77, rec:"bid",    deadlineLabel:"24 days", deadlineLevel:"soon", isNew:true,  day:"today", value:"$3.1M ceiling", valueNum:3.1 },
  { id:"o11", title:"Critical Infrastructure Threat Intel",  agency:"DHS · CISA", naics:"541690", sol:"70RCSA26R0000871", fit:73, rec:"bid",   deadlineLabel:"18 days", deadlineLevel:"soon", isNew:true,  day:"today", value:"$2.4M est.", valueNum:2.4 },
  { id:"o7", title:"AI/ML Model Evaluation Framework",      agency:"NASA · JPL", naics:"541715", sol:"80NSSC24R0102",   fit:79, rec:"bid",    deadlineLabel:"19 days", deadlineLevel:"soon", isNew:false, day:"earlier", value:"$2.7M ceiling", valueNum:2.7 },
  { id:"o8", title:"Network Operations Center Staffing",    agency:"DHS · CBP",  naics:"541513", sol:"70B04C24R0088",   fit:46, rec:"review", deadlineLabel:"31 days", deadlineLevel:"calm", isNew:false, day:"earlier", value:"$1.2M est.", valueNum:1.2 },
  { id:"o9", title:"Legacy COBOL System Modernization",     agency:"SSA",        naics:"541511", sol:"28321824R0006",   fit:68, rec:"bid",    deadlineLabel:"40 days", deadlineLevel:"calm", isNew:false, day:"earlier", value:"$5.5M ceiling", valueNum:5.5 },
  { id:"o12", title:"Grants Management System Modernization", agency:"HHS · ACF", naics:"541511", sol:"75ACF126R0044",  fit:61, rec:"review", deadlineLabel:"36 days", deadlineLevel:"calm", isNew:false, day:"earlier", value:"$4.2M ceiling", valueNum:4.2 },
];

/* Active, in-flight proposals (the command view).
   stageIndex maps into KAIMI_STAGE_NAMES; status drives the visual. */
export const KAIMI_PROPOSALS = [
  { id:"p1", title:"Zero Trust Architecture Modernization", agency:"DHS · CISA", fit:82, deadlineLabel:"9 days", deadlineLevel:"near",
    oppId:"o1", sol:"70RCSA24R0000123", value:"$4.8M", valueNum:4.8,
    stageIndex:2, status:"human",    agents:0, when:"Paused 6 min ago",  flagship:true },
  { id:"p2", title:"AI/ML Model Evaluation Framework",      agency:"NASA · JPL", fit:79, deadlineLabel:"19 days", deadlineLevel:"soon",
    oppId:"o7", sol:"80NSSC24R0102", value:"$2.7M", valueNum:2.7,
    stageIndex:2, status:"human",    agents:0, when:"Paused 31 min ago" },
  { id:"p3", title:"Cloud Migration & DevSecOps Support",   agency:"GSA · FAS",  fit:76, deadlineLabel:"22 days", deadlineLevel:"soon",
    oppId:"o2", sol:"47QTCA24R0041", value:"$2.1M", valueNum:2.1,
    stageIndex:1, status:"progress", agents:3, when:"Tomás drafting now" },
  { id:"p4", title:"Enterprise Data Platform Engineering",  agency:"VA · OIT",   fit:71, deadlineLabel:"16 days", deadlineLevel:"soon",
    oppId:"o3", sol:"36C10B24R0007", value:"$3.4M", valueNum:3.4,
    stageIndex:1, status:"progress", agents:2, when:"Tomás drafting now" },
  { id:"p5", title:"Identity & Access Management Services",  agency:"Treasury",   fit:74, deadlineLabel:"28 days", deadlineLevel:"soon",
    oppId:"o5", sol:"2032H824R00012", value:"$1.9M", valueNum:1.9,
    stageIndex:0, status:"progress", agents:1, when:"Noa outlining now" },
  { id:"p6", title:"Insider Threat Analytics Platform",     agency:"DoD · DCSA", fit:80, deadlineLabel:"34 days", deadlineLevel:"calm",
    oppId:null, sol:"HS002126R0061", value:"$5.1M", valueNum:5.1,
    stageIndex:3, status:"progress", agents:2, when:"Vera finalizing" },
];

/* Submitted archive — every proposal that went out the door.
   status: pending (awaiting award) | won | lost. valueNum in $M. */
export const KAIMI_SUBMITTED = [
  { id:"s1", title:"Continuous Diagnostics & Mitigation Support", agency:"DHS · CISA", sol:"70RCSD25R0000412", naics:"541512",
    fit:81, value:"$3.2M", valueNum:3.2, submitted:"May 22, 2026", status:"pending", award:"Award expected Jul 2026",
    docs:[ {name:"technical-volume-final.docx", meta:"21 pp"}, {name:"compliance-matrix.xlsx", meta:"31 reqs"}, {name:"price-volume.xlsx", meta:"Vol III"}, {name:"solicitation-70RCSD25R0000412.pdf", meta:"SAM.gov"} ] },
  { id:"s2", title:"Data Center Consolidation Phase II", agency:"VA · OIT", sol:"36C10B25R0099", naics:"541513",
    fit:69, value:"$2.9M", valueNum:2.9, submitted:"Apr 30, 2026", status:"pending", award:"Award expected Aug 2026",
    docs:[ {name:"technical-volume-final.docx", meta:"17 pp"}, {name:"compliance-matrix.xlsx", meta:"19 reqs"}, {name:"migration-architecture.pdf", meta:"diagram"}, {name:"solicitation-36C10B25R0099.pdf", meta:"SAM.gov"} ] },
  { id:"s3", title:"DevSecOps Pipeline Factory", agency:"USAF · Platform One", sol:"FA877525R0008", naics:"541511",
    fit:75, value:"$2.6M", valueNum:2.6, submitted:"Apr 11, 2026", status:"pending", award:"Q&A round closed",
    docs:[ {name:"technical-volume-final.docx", meta:"19 pp"}, {name:"compliance-matrix.xlsx", meta:"26 reqs"}, {name:"pipeline-reference-architecture.pdf", meta:"diagram"}, {name:"solicitation-FA877525R0008.pdf", meta:"SAM.gov"} ] },
  { id:"s4", title:"ICAM Modernization & PIV Enablement", agency:"GSA · FAS", sol:"47QTCA25R0107", naics:"541512",
    fit:84, value:"$1.8M", valueNum:1.8, submitted:"Nov 18, 2025", status:"won", award:"Awarded Jan 9, 2026",
    docs:[ {name:"technical-volume-final.docx", meta:"16 pp"}, {name:"compliance-matrix.xlsx", meta:"22 reqs"}, {name:"icam-target-architecture.pdf", meta:"diagram"}, {name:"past-performance-refs.docx", meta:"Vol II"}, {name:"solicitation-47QTCA25R0107.pdf", meta:"SAM.gov"} ] },
  { id:"s5", title:"Cloud Security Baseline & ATO Acceleration", agency:"DoD · DISA", sol:"HC102825R0054", naics:"541512",
    fit:78, value:"$1.6M", valueNum:1.6, submitted:"Oct 2, 2025", status:"won", award:"Awarded Dec 15, 2025",
    docs:[ {name:"technical-volume-final.docx", meta:"14 pp"}, {name:"compliance-matrix.xlsx", meta:"18 reqs"}, {name:"ato-evidence-framework.pdf", meta:"diagram"}, {name:"solicitation-HC102825R0054.pdf", meta:"SAM.gov"} ] },
  { id:"s6", title:"Secure SD-WAN Implementation", agency:"USDA", sol:"12314825R0071", naics:"541513",
    fit:62, value:"$1.4M", valueNum:1.4, submitted:"Sep 12, 2025", status:"lost", award:"Not awarded · debrief on file",
    docs:[ {name:"technical-volume-final.docx", meta:"15 pp"}, {name:"compliance-matrix.xlsx", meta:"17 reqs"}, {name:"award-debrief-notes.docx", meta:"lessons"}, {name:"solicitation-12314825R0071.pdf", meta:"SAM.gov"} ] },
];

/* The flagship proposal's review detail (used in the workspace). */
export const KAIMI_REVIEW = {
  fromAgent: "writer",
  prompt: "Tomás finished the technical volume and flagged one gap. Review it before the final pass runs.",
  summary: "Drafted the full technical volume — 18 pages across all 7 sections, in CISA's required format, with a compliance matrix mapping every requirement.",
  criteria: [
    { label:"All 24 requirements addressed", state:"warn", note:"22 of 24 — 2 need past-performance evidence" },
    { label:"CISA formatting & page limits",  state:"ok" },
    { label:"Win themes in each section",      state:"ok" },
    { label:"Past-performance sufficiency",    state:"warn", note:"Flagged below" },
  ],
  gap: {
    title:"No past-performance for cybersecurity at this scale",
    detail:"Draft cites two relevant contracts, but neither exceeds $2M. This solicitation weights past performance heavily — recommend a teaming partner before submission.",
  },
  artifacts: [
    { name:"technical-volume-draft-v3.docx", meta:"18 pp" },
    { name:"compliance-matrix.xlsx", meta:"24 reqs" },
  ],
};
