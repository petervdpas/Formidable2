// Package credential is a thin wrapper around the OS-native keychain
// (macOS Keychain / Linux Secret Service / Windows Credential
// Manager) via zalando/go-keyring.
//
// Lives under collaboration because it's shared infrastructure
// between the Git and (future) GiGot backends — both of which need
// to persist a PAT or token across sessions without writing it to
// disk in plaintext.
//
// Account naming: Git uses the remote URL ("https://github.com/foo
// /bar.git") so a PAT is bound to the repo it was issued for.
// GiGot will use its own scheme when wired. The service name
// ("Formidable") is the keychain "vendor" namespace shared by all
// backends so all credentials show up grouped in the OS UI.
package credential

// Has-result for "is there an entry for this account" queries.
// Wails-bindable; a bare bool would also work but a struct future-
// proofs metadata like the timestamp of last update.
type LookupResult struct {
	Found bool `json:"found"`
}
