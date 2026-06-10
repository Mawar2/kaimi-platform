/* ============================================================
   KAIMI — App data · opportunity queue + active proposals.
   Ported from the design handoff (app-data.js). The opportunity
   queue here is the fallback shown when the local store is empty;
   App.jsx replaces it with real store data when present (see api.js).
   Proposals/Workspace stay on this mock data until Zone 2 agent
   events are wired in a later phase (per INTENT.md).
   ============================================================ */

export const KAIMI_STAGE_NAMES = ["Outline","Technical Writer","Human Review","Final Review","Submit"];

export const KAIMI_AGENTS = {
  outline: { name:"Noa",   role:"Outline",          initial:"N", hue:"blue"   },
  writer:  { name:"Tomás", role:"Technical Writer", initial:"T", hue:"cyan"   },
  review:  { name:"Vera",  role:"Final Review",     initial:"V", hue:"violet" },
};

/* The SAM.gov opportunity queue (Triage) — fallback / demo data. */
export const KAIMI_OPPS = [
  { id:"o1", title:"Zero Trust Architecture Modernization", agency:"DHS · CISA", naics:"541512", sol:"70RCSA24R0000123", fit:82, rec:"bid",    deadlineLabel:"9 days",  deadlineLevel:"near", isNew:true,  day:"today", value:"$4.8M ceiling" },
  { id:"o2", title:"Cloud Migration & DevSecOps Support",   agency:"GSA · FAS",  naics:"541512", sol:"47QTCA24R0041",   fit:76, rec:"bid",    deadlineLabel:"22 days", deadlineLevel:"soon", isNew:true,  day:"today", value:"$2.1M est." },
  { id:"o3", title:"Enterprise Data Platform Engineering",  agency:"VA · OIT",   naics:"541511", sol:"36C10B24R0007",   fit:71, rec:"bid",    deadlineLabel:"16 days", deadlineLevel:"soon", isNew:true,  day:"today", value:"$3.4M ceiling" },
  { id:"o4", title:"SOC Modernization & Threat Hunting",    agency:"DoD · DISA", naics:"541519", sol:"HC102824R0019",   fit:64, rec:"review", deadlineLabel:"6 days",  deadlineLevel:"crit", isNew:true,  day:"today", value:"$6.2M ceiling" },
  { id:"o5", title:"Identity & Access Management Services",  agency:"Treasury",   naics:"541512", sol:"2032H824R00012",  fit:58, rec:"review", deadlineLabel:"28 days", deadlineLevel:"soon", isNew:true,  day:"today", value:"$1.9M est." },
  { id:"o6", title:"Help Desk & End-User IT Support",       agency:"USDA",       naics:"541513", sol:"12314824R0033",   fit:34, rec:"nobid",  deadlineLabel:"11 days", deadlineLevel:"near", isNew:true,  day:"today", value:"$850K est." },
  { id:"o7", title:"AI/ML Model Evaluation Framework",      agency:"NASA · JPL", naics:"541715", sol:"80NSSC24R0102",   fit:79, rec:"bid",    deadlineLabel:"19 days", deadlineLevel:"soon", isNew:false, day:"earlier", value:"$2.7M ceiling" },
  { id:"o8", title:"Network Operations Center Staffing",    agency:"DHS · CBP",  naics:"541513", sol:"70B04C24R0088",   fit:46, rec:"review", deadlineLabel:"31 days", deadlineLevel:"calm", isNew:false, day:"earlier", value:"$1.2M est." },
  { id:"o9", title:"Legacy COBOL System Modernization",     agency:"SSA",        naics:"541511", sol:"28321824R0006",   fit:68, rec:"bid",    deadlineLabel:"40 days", deadlineLevel:"calm", isNew:false, day:"earlier", value:"$5.5M ceiling" },
];

/* Active, in-flight proposals (the command view).
   stageIndex maps into KAIMI_STAGE_NAMES; status drives the visual. */
export const KAIMI_PROPOSALS = [
  { id:"p1", title:"Zero Trust Architecture Modernization", agency:"DHS · CISA", fit:82, deadlineLabel:"9 days", deadlineLevel:"near",
    stageIndex:2, status:"human",    agents:0, when:"Paused 6 min ago",  flagship:true },
  { id:"p2", title:"AI/ML Model Evaluation Framework",      agency:"NASA · JPL", fit:79, deadlineLabel:"19 days", deadlineLevel:"soon",
    stageIndex:2, status:"human",    agents:0, when:"Paused 31 min ago" },
  { id:"p3", title:"Cloud Migration & DevSecOps Support",   agency:"GSA · FAS",  fit:76, deadlineLabel:"22 days", deadlineLevel:"soon",
    stageIndex:1, status:"progress", agents:3, when:"Tomás drafting now" },
  { id:"p4", title:"Enterprise Data Platform Engineering",  agency:"VA · OIT",   fit:71, deadlineLabel:"16 days", deadlineLevel:"soon",
    stageIndex:1, status:"progress", agents:2, when:"Tomás drafting now" },
  { id:"p5", title:"Identity & Access Management Services",  agency:"Treasury",   fit:74, deadlineLabel:"28 days", deadlineLevel:"soon",
    stageIndex:0, status:"progress", agents:1, when:"Noa outlining now" },
  { id:"p6", title:"Insider Threat Analytics Platform",     agency:"DoD · DCSA", fit:80, deadlineLabel:"34 days", deadlineLevel:"calm",
    stageIndex:3, status:"progress", agents:2, when:"Vera finalizing" },
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
