package vault

import "strings"

// Resolver is a single category's view of a vault, presented as the one-method
// secret lookup consumers such as the api-client invoker expect. It is
// deliberately structural: neither package imports the other, so the vault
// stays a general store and the invoker keeps knowing nothing about crypto.
//
// The category is the namespace, so an api client named "northwind" cannot
// collide with a git remote of the same name in the same vault.
type Resolver struct {
	Category string

	// get is resolved per call, not captured once. The composition root wires a
	// resolver at startup, before the user has created or unlocked anything, so
	// binding the vault eagerly would pin a nil forever.
	get func() *Vault
}

// NewResolver returns a Resolver over a vault that already exists.
func NewResolver(v *Vault, category string) *Resolver {
	return &Resolver{Category: category, get: func() *Vault { return v }}
}

// NewLazyResolver returns a Resolver that asks for the vault on every call, so
// it survives the vault being created, unlocked, or relocked later.
func NewLazyResolver(get func() *Vault, category string) *Resolver {
	return &Resolver{Category: category, get: get}
}

// Vault returns the vault behind this resolver, or nil when there is none yet.
func (r *Resolver) Vault() *Vault {
	if r == nil || r.get == nil {
		return nil
	}
	return r.get()
}

func (r *Resolver) catalog(id string) (*Catalog, string, error) {
	v := r.Vault()
	if v == nil {
		return nil, "", ErrVaultNotFound
	}
	key := strings.TrimSpace(id)
	if key == "" || strings.TrimSpace(r.Category) == "" {
		return nil, "", ErrInvalidName
	}
	return NewCatalog(v), key, nil
}

// Secret returns the stored credential for id. A locked vault, a missing entry
// and a corrupt record each surface as their own error, so the caller can tell
// "unlock first" from "nothing stored yet".
func (r *Resolver) Secret(id string) (string, error) {
	c, key, err := r.catalog(id)
	if err != nil {
		return "", err
	}
	return c.Get(r.Category, key)
}

// Store writes the credential for id, creating or replacing it.
func (r *Resolver) Store(id, secret string) error {
	c, key, err := r.catalog(id)
	if err != nil {
		return err
	}
	_, err = c.Put(r.Category, key, secret, "")
	return err
}

// Forget removes the credential for id. Removing one that was never stored is
// reported as ErrNotFound so a caller can tell it from a real deletion.
func (r *Resolver) Forget(id string) error {
	c, key, err := r.catalog(id)
	if err != nil {
		return err
	}
	return c.Delete(r.Category, key)
}

// Has reports whether a credential is stored for id.
//
// This needs the vault unlocked. Opaque slots put identity inside the
// ciphertext, so a locked vault cannot answer and returns false rather than
// guessing. Callers that wanted to show configured state before the password
// prompt no longer can: that is the price of a directory that leaks nothing.
func (r *Resolver) Has(id string) bool {
	c, key, err := r.catalog(id)
	if err != nil {
		return false
	}
	return c.Has(r.Category, key)
}

// IDs lists the ids this resolver holds credentials for. Requires an unlocked
// vault, for the same reason Has does.
func (r *Resolver) IDs() ([]string, error) {
	v := r.Vault()
	if v == nil {
		return nil, ErrVaultNotFound
	}
	entries, err := NewCatalog(v).List()
	if err != nil {
		return nil, err
	}
	want := foldIdentity(r.Category)
	var out []string
	for _, e := range entries {
		if foldIdentity(e.Category) == want {
			out = append(out, e.Key)
		}
	}
	return out, nil
}
