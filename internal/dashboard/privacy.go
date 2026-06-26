package dashboard

import "net/http"

// handlePrivacy serves GET /privacy: the public, UNGATED privacy policy. It is mounted
// outside the product-key wrap (see internal/httpapi server wiring) so it is reachable
// without a session — a requirement for Google's OAuth consent-screen verification, which
// needs a publicly accessible privacy-policy URL on the app's own domain. Self-contained
// (inline CSS, no external assets) so it renders under the strict CSP.
func (h *Handler) handlePrivacy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(privacyHTML))
}

// privacyHTML is the standalone privacy policy. It is written to be ACCURATE to what Kaimi
// actually does with data (verified against the codebase) and to satisfy Google's OAuth
// verification requirements, including the verbatim Limited Use affirmation. Keep the
// "Last updated" date current whenever data practices change.
const privacyHTML = `<!doctype html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Kaimi — Privacy Policy</title>
<style>
:root{--bg:#0a0f1c;--surface:#0e1525;--border:#1e2a44;--ink:#e8eefc;--ink2:#aebbd6;--ink3:#7d8aa8;--blue:#3b82f6;--ok:#22c55e;}
*{box-sizing:border-box;margin:0;padding:0}
body{background:var(--bg);color:var(--ink);font:15px/1.65 -apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;padding:32px 16px;}
.wrap{max-width:760px;margin:0 auto;}
.brand{display:flex;align-items:center;gap:12px;margin-bottom:8px;}
.mark{width:36px;height:36px;border-radius:9px;background:linear-gradient(135deg,#3b82f6,#22b8cf);display:flex;align-items:center;justify-content:center;font-size:20px;font-weight:700;}
.brand h1{font-size:20px;} .brand .tag{font-size:11px;letter-spacing:.08em;text-transform:uppercase;color:var(--ink3);}
h2{font-size:17px;margin:28px 0 10px;padding-bottom:6px;border-bottom:1px solid var(--border);}
h3{font-size:14px;margin:16px 0 6px;color:var(--ink);}
p,li{color:var(--ink2);} .lead{color:var(--ink2);margin:10px 0 4px;}
ol,ul{margin:8px 0 8px 22px;} li{margin:6px 0;}
.card{background:var(--surface);border:1px solid var(--border);border-radius:10px;padding:16px 18px;margin:14px 0;}
code{background:#11192c;border:1px solid var(--border);border-radius:5px;padding:1px 6px;font-family:ui-monospace,Menlo,Consolas,monospace;font-size:13px;color:#9ec1ff;}
a{color:#7cb3ff;} .note{font-size:13px;color:var(--ink3);}
.back{display:inline-block;margin-top:26px;color:#7cb3ff;text-decoration:none;}
strong{color:var(--ink);}
</style></head>
<body><div class="wrap">
  <div class="brand"><span class="mark">&#8776;</span><div><h1>Kaimi</h1><div class="tag">Privacy Policy</div></div></div>
  <p class="lead"><strong>Last updated:</strong> June 26, 2026</p>
  <p class="lead">This Privacy Policy explains how Kaimi, operated by <strong>BlueMeta Technologies</strong> ("we," "us"), handles information when you use the Kaimi application. Questions: <a href="mailto:malik@bluemetatech.com">malik@bluemetatech.com</a>.</p>

  <h2>What Kaimi is</h2>
  <p>Kaimi is a business-development tool for U.S. federal contractors. It finds relevant federal opportunities from <a href="https://sam.gov" target="_blank" rel="noopener">SAM.gov</a>, scores them against your company profile, and drafts proposal documents for your review. Kaimi never submits anything on your behalf — a human always reviews and decides.</p>

  <h2>Information we collect</h2>
  <div class="card">
  <h3>Information you provide</h3>
  <ul>
    <li><strong>Access credential.</strong> Access is granted through a private access link (a "product key"). If you invite a teammate, we store the email address you enter so the invitation can be labeled and revoked.</li>
    <li><strong>Company profile.</strong> The details you enter during setup: company name, UEI/CAGE identifiers, NAICS codes, small-business / set-aside eligibility, capability statements, and past-performance records (and, optionally, address or clearance level).</li>
    <li><strong>SAM.gov API key.</strong> Your own SAM.gov key, which Kaimi uses solely to query SAM.gov on your behalf.</li>
    <li><strong>Documents you upload.</strong> Capability statements, past proposals, and solicitation files you add for context.</li>
  </ul>
  <h3>Information Kaimi generates or retrieves</h3>
  <ul>
    <li><strong>Federal opportunity data</strong> retrieved from the public SAM.gov API, and the proposal drafts Kaimi produces for you.</li>
  </ul>
  <h3>Google account information (only if you connect Google Drive)</h3>
  <p>Connecting Google Drive is <strong>optional</strong>. If you connect it, Kaimi requests only the <code>drive.file</code> scope. This means:</p>
  <ul>
    <li>Kaimi can <strong>only create and manage the files it creates</strong> in your Drive — the "Kaimi Proposals" folder and the proposal documents it generates for you.</li>
    <li>Kaimi <strong>cannot see, read, list, or access any other files</strong> in your Google Drive.</li>
    <li>To do this, Google provides Kaimi an authorization token, which we store securely on the server and never display or log.</li>
  </ul>
  </div>

  <h2>How we use information</h2>
  <ul>
    <li>To run the workflow you asked for: search SAM.gov (using your key), score and explain opportunities against your profile, and draft and review proposals.</li>
    <li>To save proposals to your Google Drive — but <strong>only</strong> when you choose "Save to Google Drive," and only by creating/updating the documents Kaimi itself creates there.</li>
    <li>We do <strong>not</strong> use your information for advertising, and we do <strong>not</strong> sell it.</li>
  </ul>

  <h2>AI processing</h2>
  <p>To score opportunities and draft proposals, Kaimi sends relevant content — opportunity text, your company profile, and proposal drafts — to AI models hosted on <strong>Google Cloud Vertex AI</strong> (Google's Gemini models and Anthropic's Claude models accessed through Vertex AI). Your information — including any Google user data — is <strong>not used to develop, train, or improve generalized or non-personalized AI/ML models</strong>. It is processed only to provide Kaimi's features to you.</p>

  <h2>How we share information</h2>
  <p>We do not sell your information. We share it only with service providers that operate Kaimi, and only as needed to provide the service:</p>
  <ul>
    <li><strong>Google Cloud Platform</strong> — hosting, Vertex AI (Gemini), Document AI (extracting text from solicitation files), Cloud Storage, Secret Manager, Firestore, and logging.</li>
    <li><strong>Anthropic</strong> — Claude AI models, accessed through Google Cloud Vertex AI.</li>
    <li><strong>SAM.gov (U.S. Government)</strong> — receives your API key and search parameters to return federal opportunities.</li>
  </ul>
  <p>We may also disclose information if required by law or to protect the security and integrity of the service.</p>

  <h2>Google user data &mdash; Limited Use</h2>
  <div class="card">
  <p>Kaimi's use and transfer of information received from Google APIs to any other app will adhere to the <a href="https://developers.google.com/terms/api-services-user-data-policy" target="_blank" rel="noopener">Google API Services User Data Policy</a>, including the Limited Use requirements.</p>
  <p class="note">In particular, data obtained from Google Drive is used only to provide the proposal-document feature you requested; is not transferred to others except as necessary to provide that feature, for security, or to comply with law; is never used for advertising; is not read by humans except where you direct it, where required for security, or where required by law; and is never used to train generalized AI/ML models.</p>
  </div>

  <h2>How we store and protect information</h2>
  <ul>
    <li>Your data is hosted in a dedicated Google Cloud project and is <strong>encrypted at rest</strong> by Google Cloud's infrastructure; data in transit is protected with <strong>HTTPS/TLS</strong>.</li>
    <li>Your <strong>SAM.gov API key</strong> is stored in Google Secret Manager; your <strong>Google Drive authorization token</strong> is stored server-side with restricted access. Neither is ever displayed in the app or written to logs.</li>
    <li>Sessions use hardened, signed cookies, and credentials/tokens are excluded from logs.</li>
  </ul>

  <h2>Retention and deletion</h2>
  <ul>
    <li>We retain your information for as long as your Kaimi account is active.</li>
    <li>You can <strong>disconnect Google Drive at any time</strong> from within Kaimi, or revoke Kaimi's access directly in your Google Account at <a href="https://myaccount.google.com/permissions" target="_blank" rel="noopener">myaccount.google.com/permissions</a>. Revoking access stops Kaimi from accessing your Drive going forward; documents Kaimi already created remain yours in your Drive.</li>
    <li>To request deletion of your Kaimi data, email <a href="mailto:malik@bluemetatech.com">malik@bluemetatech.com</a> and we will delete it.</li>
  </ul>

  <h2>Your choices</h2>
  <p>Connecting Google Drive is optional — you can use Kaimi and export proposals as Word or PDF files without connecting Drive at all.</p>

  <h2>Changes to this policy</h2>
  <p>If we change how we handle your information — including Google user data — we will update the "Last updated" date above and, where appropriate, notify you.</p>

  <h2>Contact</h2>
  <p>BlueMeta Technologies &middot; <a href="mailto:malik@bluemetatech.com">malik@bluemetatech.com</a></p>

  <a class="back" href="/help">&larr; Back to Help</a>
</div></body></html>`
