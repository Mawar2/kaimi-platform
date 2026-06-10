/* ============================================================
   KAIMI — Proposal Lifecycle · mock data + state machine
   The one selected proposal the agents are working.
   ============================================================ */
window.KAIMI_PROPOSAL = {
  title: "Zero Trust Architecture Modernization",
  agency: "Dept. of Homeland Security · CISA",
  naics: "541512",
  sol: "70RCSA24R0000123",
  fit: 82,
  rec: "bid",
  deadlineLabel: "9 days",
  deadlineLevel: "near",
  value: "$4.8M ceiling · 36-month IDIQ",
  selectedAgo: "Selected 6 min ago",
};

// Named agent teammates
window.KAIMI_AGENTS = {
  outline: { name: "Noa",   role: "Outline",          initial: "N", hue: "blue" },
  writer:  { name: "Tomás", role: "Technical Writer",  initial: "T", hue: "cyan" },
  review:  { name: "Vera",  role: "Final Review",      initial: "V", hue: "violet" },
};

// The pipeline stages, in order.
window.KAIMI_STAGES = [
  {
    id: "outline", kind: "agent", agentKey: "outline",
    name: "Outline",
    working: "Mapping the solicitation into a section plan…",
    summary: "Structured the proposal into 7 sections, each mapped to an RFP requirement and evaluation factor.",
    artifacts: [
      { name: "outline.md", meta: "7 sections" },
      { name: "requirements-matrix.csv", meta: "24 reqs" },
    ],
    flags: [],
    metrics: [ {k:"Sections", v:"7"}, {k:"Reqs mapped", v:"24/24"} ],
  },
  {
    id: "writer", kind: "agent", agentKey: "writer",
    name: "Technical Writer",
    working: "Drafting the technical volume from the outline…",
    summary: "Drafted the full technical volume — 18 pages across all 7 sections, in CISA's required format, with a compliance matrix.",
    artifacts: [
      { name: "technical-volume-draft-v3.docx", meta: "18 pp" },
      { name: "compliance-matrix.xlsx", meta: "24 reqs" },
    ],
    flags: [
      {
        level: "human",
        title: "No past-performance for cybersecurity at this scale",
        detail: "Draft cites two relevant contracts, but neither exceeds $2M. This solicitation weights past performance heavily — recommend a teaming partner before submission.",
        action: "Find teaming partner",
      },
    ],
    metrics: [ {k:"Pages", v:"18"}, {k:"Compliance", v:"22/24"}, {k:"Win themes", v:"4"} ],
  },
  {
    id: "gate", kind: "gate",
    name: "Human Review",
    fromAgentKey: "writer",
    prompt: "Tomás finished the technical volume and flagged a gap. Review the draft before the final pass runs.",
    criteria: [
      { label: "All 24 requirements addressed", state: "warn", note: "22 of 24 — 2 need past-performance evidence" },
      { label: "CISA formatting & page limits", state: "ok" },
      { label: "Win themes present in each section", state: "ok" },
      { label: "Past-performance sufficiency", state: "warn", note: "Flagged — see gap above" },
    ],
  },
  {
    id: "review", kind: "agent", agentKey: "review",
    name: "Final Review",
    working: "Running the final compliance and consistency pass…",
    summary: "Final compliance pass complete — 24/24 requirements addressed, formatting validated, cross-references resolved. Package is submission-ready.",
    artifacts: [
      { name: "technical-volume-FINAL.pdf", meta: "20 pp" },
      { name: "compliance-report.pdf", meta: "24/24" },
    ],
    flags: [],
    metrics: [ {k:"Compliance", v:"24/24"}, {k:"Issues", v:"0"} ],
  },
  {
    id: "submit", kind: "terminal",
    name: "Ready to Submit",
    summary: "All stages complete. The package is ready for human submission to SAM.gov.",
  },
];
