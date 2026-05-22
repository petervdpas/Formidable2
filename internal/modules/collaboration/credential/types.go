// Package credential is a thin wrapper around the OS-native keychain
// (macOS Keychain / Linux Secret Service / Windows Credential
// Manager) via zalando/go-keyring.
//
// Lives under collaboration because it's shared infrastructure
// between the Git and (future) GiGot backends - both of which need
// to persist a PAT or token across sessions without writing it to
// disk in plaintext.
//
// # Account naming convention
//
// All callers - both Vue and any future backend sync ops - must
// produce keychain account strings of the form:
//
//	<profile_filename>:<backend>:<identifier>
//
// where:
//   - profile_filename is the active profile's basename (e.g.
//     "default.json"). Profile namespacing keeps "personal" and
//     "work" tokens separate even when both reference the same
//     remote repo.
//   - backend is "git" or "gigot".
//   - identifier is per-backend: a remote URL for Git, a repo
//     name for GiGot.
//
// The frontend implements this in
// frontend/src/composables/useCredentialAccount.ts; keep both
// definitions aligned if either side changes.
//
// The keychain "service" name ("Formidable") is shared by all
// entries so they show up grouped in the OS UI.
package credential

// Has-result for "is there an entry for this account" queries.
// Wails-bindable; a bare bool would also work but a struct future-
// proofs metadata like the timestamp of last update.
type LookupResult struct {
	Found bool `json:"found"`
}
