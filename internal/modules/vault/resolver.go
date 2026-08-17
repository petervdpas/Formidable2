package vault

import "strings"

// Resolver adapts a Vault to the one-method secret lookup that consumers such
// as the api-client invoker expect. It is deliberately structural: neither
// package imports the other, so the vault stays a general store and the
// invoker keeps knowing nothing about crypto.
//
// Prefix namespaces the entries, so several subsystems can share one vault
// without colliding: an api client named "northwind" under the prefix
// "api-client-" lives at "api-client-northwind".
type Resolver struct {
	Prefix string

	// get is resolved per call, not captured once. The composition root wires
	// a resolver at startup, before the user has created or unlocked anything,
	// so binding the vault eagerly would pin a nil forever.
	get func() *Vault
}

// NewResolver returns a Resolver over a vault that already exists.
func NewResolver(v *Vault, prefix string) *Resolver {
	return &Resolver{Prefix: prefix, get: func() *Vault { return v }}
}

// NewLazyResolver returns a Resolver that asks for the vault on every call, so
// it survives the vault being created, unlocked, or relocked later.
func NewLazyResolver(get func() *Vault, prefix string) *Resolver {
	return &Resolver{Prefix: prefix, get: get}
}

// Vault returns the vault behind this resolver, or nil when there is none yet.
func (r *Resolver) Vault() *Vault {
	if r == nil || r.get == nil {
		return nil
	}
	return r.get()
}

// Name is the vault entry backing id. Exported so a settings screen can show
// the user exactly which entry a client reads, rather than making them guess.
func (r *Resolver) Name(id string) string {
	return r.Prefix + strings.TrimSpace(id)
}

// entry is Name with the empty case refused. Without this an empty id would
// resolve to the bare prefix, quietly reading and writing one shared slot that
// every caller with a blank id would collide on.
func (r *Resolver) entry(id string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", ErrInvalidName
	}
	return r.Name(id), nil
}

// Secret returns the stored credential for id. A locked vault, a missing
// entry, and a corrupt record all surface as their own error, so the caller
// can tell "unlock first" from "nothing stored yet".
func (r *Resolver) Secret(id string) (string, error) {
	v := r.Vault()
	if v == nil {
		return "", ErrVaultNotFound
	}
	name, err := r.entry(id)
	if err != nil {
		return "", err
	}
	return v.Get(name)
}

// Store writes the credential for id, creating or replacing it.
func (r *Resolver) Store(id, secret string) error {
	v := r.Vault()
	if v == nil {
		return ErrVaultNotFound
	}
	name, err := r.entry(id)
	if err != nil {
		return err
	}
	return v.Set(name, secret)
}

// Forget removes the credential for id. Removing one that was never stored is
// reported as ErrNotFound so a caller can distinguish it from a real deletion.
func (r *Resolver) Forget(id string) error {
	v := r.Vault()
	if v == nil {
		return ErrVaultNotFound
	}
	name, err := r.entry(id)
	if err != nil {
		return err
	}
	return v.Delete(name)
}

// Has reports whether a credential is stored for id. It needs no unlock,
// because entry names are filenames, so a settings screen can show which
// clients are configured before the user has typed the master password.
func (r *Resolver) Has(id string) bool {
	v := r.Vault()
	if v == nil {
		return false
	}
	name, err := r.entry(id)
	if err != nil {
		return false
	}
	return v.Has(name)
}

// IDs lists the ids this resolver holds credentials for, with the prefix
// stripped. Also unlock-free.
func (r *Resolver) IDs() ([]string, error) {
	v := r.Vault()
	if v == nil {
		return nil, ErrVaultNotFound
	}
	names, err := v.List()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, name := range names {
		if id, ok := strings.CutPrefix(name, r.Prefix); ok && id != "" {
			out = append(out, id)
		}
	}
	return out, nil
}
