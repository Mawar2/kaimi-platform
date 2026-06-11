// Package drivetoken persists a deployment's connection to the CUSTOMER's own
// Google Workspace/Drive and turns it into an oauth2.TokenSource that the
// googledocs client uses to write proposal Docs into THAT Drive (WS-C2).
//
// It holds two server-side artifacts, both rooted at the same store base path the
// opportunity and profile stores use (this deployment is single-tenant today):
//
//   - The OAuth token (access + refresh) obtained from the customer's Drive
//     consent flow, stored atomically at <basePath>/drive_token.json with
//     owner-only (0o600) permissions. The token is a SECRET: it is never logged.
//   - The target Drive/folder id where Docs should be created, stored at
//     <basePath>/drive_target.json. (The interactive Drive picker is WS-C3; here a
//     target id is simply persisted once provided.)
//
// TokenSourceFromStore wraps the stored token in an oauth2.Config-backed
// TokenSource so an expired access token auto-refreshes from the refresh token
// without re-prompting the user. The OAuth config requests the MINIMAL scopes —
// drive.file (files the app creates) and documents — never the full-drive scope.
//
// The JSON-file implementations mirror internal/profile.JSONProfileStore's
// temp-file + atomic-rename pattern so they can later swap to a Secret
// Manager/GCS/Firestore backing without touching callers.
package drivetoken
