# Capability-map scoring A/B evaluation

**Company:** Ey3 Technologies · **Scorer:** gemini-2.5-pro (Vertex) · **Opportunities:** 8

- **Arm A (current):** scores the raw `Description` (a SAM `noticedesc` URL) with profile-only signals.
- **Arm B (capability-map):** scores the RESOLVED solicitation text with capability-map coverage signals.

**Headline:** 7/8 descriptions resolved to text; net score delta (B−A) = -10 total; recommendation changed on 0 opportunities.

| Opportunity | A score/rec | B score/rec | Δ | B matched capabilities |
|---|---|---|---|---|
| MAMMOTH-CSO | 5 / NO_BID | 0 / NO_BID | -5 |  |
| Multiple Award Schedule | 5 / NO_BID | 0 / NO_BID | -5 |  |
| NATO Business Opportunity: Gamification Solutio… | 5 / NO_BID | 5 / NO_BID | +0 |  |
| DHS Network Operations Security Center (NOSC) N… | 5 / NO_BID | 5 / NO_BID | +0 | Homeland Security, DHS |
| NATO Business Opportunity: Tier 2 Tools and Sec… | 5 / NO_BID | 5 / NO_BID | +0 |  |
| Endpoint Security Event Management | 5 / NO_BID | 5 / NO_BID | +0 |  |
| VA OIG CyberFeds Renewal | 5 / NO_BID | 0 / NO_BID | -5 | 541519 |
| NATO Business Opportunity: AI-Assisted Tabletop… | 0 / NO_BID | 5 / NO_BID | +5 |  |

## Per-opportunity notes

- **MAMMOTH-CSO** (`03f95f31`): A=5/NO_BID → B=0/NO_BID.
- **Multiple Award Schedule** (`085e2cdd`): A=5/NO_BID → B=0/NO_BID. _resolve failed: samgov: description fetch https://api.sam.gov/prod/opportunities/v1/noticedesc?api_key=REDACTED&noticeid=085e2cdd9d8740f796b9f26dfff73c00 returned status 404_
- **NATO Business Opportunity: Gamification Solutions for Cyberspace Warfare Development** (`20162a4d`): A=5/NO_BID → B=5/NO_BID.
- **DHS Network Operations Security Center (NOSC) Network, Cloud, and Cyber Services (NCCS) 2.0 Industry Day** (`5c98944b`): A=5/NO_BID → B=5/NO_BID.
- **NATO Business Opportunity: Tier 2 Tools and Security Systems Uplift** (`7105948c`): A=5/NO_BID → B=5/NO_BID.
- **Endpoint Security Event Management** (`f52068ae`): A=5/NO_BID → B=5/NO_BID.
- **VA OIG CyberFeds Renewal** (`f9ef06b3`): A=5/NO_BID → B=0/NO_BID.
- **NATO Business Opportunity: AI-Assisted Tabletop Exercise (TTX) Framework for Cyberspace Operations** (`fc21b06e`): A=0/NO_BID → B=5/NO_BID.

_Generated 2026-06-24 23:18 UTC. One-off eval — not wired into the live pipeline. Cutover is Malik's decision._

---

## Interpretation & recommendation (for Malik)

**What worked**
- **Description resolution is real and works (7/8).** Today's scorer scores `Description`, which is a SAM `noticedesc` **URL, not text** — so it literally scores a link. The resolver fetched real solicitation text for 7 of 8 opps (1 stale v1 URL 404'd; it degrades gracefully). This is an unambiguous correctness fix.
- **The capability-map signal surfaces explainable matches** — e.g. the DHS NOSC opp matched `Homeland Security`, `DHS`; VA OIG matched NAICS `541519`. That rationale is now visible to the scorer (and already to the user, via phase D).

**What the numbers actually say (be honest)**
- **Absolute scores are floored: 0–10, every opp NO_BID in both arms.** That is the **thin TEST profile** (the Ey3 placeholder I created during the build — minimal NAICS/competencies/past-perf), not a verdict on the map. A real, fully-onboarded tenant profile would score very differently.
- **Run-to-run LLM variance (±5/opp) exceeds the map's effect on this sample.** Two identical runs gave net deltas of **+0** and **−10** with **0 recommendation changes** both times. At these floored scores, the capability-map signal does **not** produce a reliable, measurable score/recommendation improvement — the profile richness and rubric dominate.
- **Conclusion: this eval validates the *mechanism* (resolution + map signal plumb through correctly and are explainable), but does NOT, on a thin test profile, demonstrate a scoring *win*.** Drawing a cutover conclusion from it would be over-reading noise.

**Recommendation — split the decision:**
1. **Ship now (low risk, clearly correct):** wire the **description resolver + `EffectiveDescription()`** into the live pipeline for the *eligible (post-gate) set only* (SAM-quota-bounded). Scoring real solicitation text instead of a URL is unambiguously better and is already additive/safe — independent of the map question.
2. **Do NOT enable the capability-map scoring signal in production yet.** First re-run this A/B on a **real, fully-onboarded profile** (not the test placeholder) AND review the scorer rubric/thresholds (the uniform NO_BID flooring says the rubric and/or profile richness — not the map — is the dominant lever). Decide on the map signal after that.

**Nothing here is wired into the live scorer.** The production pipeline is unchanged; `EffectiveDescription()` is a no-op until a description is resolved, and the map signal is opt-in (nil in the live path). Your call on (1) and (2).
