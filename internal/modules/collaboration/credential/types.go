// Package credential wraps the OS-native keychain (macOS Keychain / Linux Secret
// Service / Windows Credential Manager) via zalando/go-keyring, shared between the
// Git and GiGot backends to persist a PAT or token without writing it to disk in plaintext.
//
// # Account naming convention
//
// All callers must produce keychain account strings of the form:
//
//	<profile_filename>:<backend>:<identifier>
//
// where:
//   - profile_filename is the active profile basename; profile namespacing keeps
//     "personal" and "work" tokens separate even against the same remote.
//   - backend is "git" or "gigot".
//   - identifier is per-backend: a remote URL for Git, a repo name for GiGot.
//
// The frontend mirrors this in frontend/src/composables/useCredentialAccount.ts;
// keep both definitions aligned if either side changes.
package credential

// LookupResult is the Has-query result.
type LookupResult struct {
	Found bool `json:"found"`
}
