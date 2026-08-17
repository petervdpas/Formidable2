package vault

import (
	"cmp"
	"errors"
	"slices"
	"strings"
	"time"
)

// Catalog is the secret store as callers see it: a set of (category, key)
// pairs. It sits on top of Vault, which stays a plain encrypted name/value
// store, so the crypto core is unchanged and the opaque-slot decision lives in
// exactly one place.
//
// Cost of opaque slots: identity is inside the ciphertext, so listing or
// finding a secret means decrypting every record. That is the price of a
// directory that reveals nothing, and at the scale a personal vault runs at it
// is not worth optimising away.
type Catalog struct {
	v *Vault
}

// NewCatalog wraps a vault. A nil vault yields a catalog whose operations all
// report ErrVaultNotFound rather than panicking.
func NewCatalog(v *Vault) *Catalog { return &Catalog{v: v} }

// Vault returns the underlying store.
func (c *Catalog) Vault() *Vault {
	if c == nil {
		return nil
	}
	return c.v
}

// CatalogEntry is one secret's identity and metadata, without its value.
type CatalogEntry struct {
	Slot        string    `json:"slot"`
	Category    string    `json:"category"`
	Key         string    `json:"key"`
	Description string    `json:"description,omitempty"`
	CreatedUTC  time.Time `json:"created_utc"`
	UpdatedUTC  time.Time `json:"updated_utc"`
}

// Put stores a value under (category, key), replacing any existing one. The
// slot name and creation time are preserved across a replace, so rewriting a
// secret does not look like deleting and re-adding it.
func (c *Catalog) Put(category, key, value, description string) (CatalogEntry, error) {
	if err := checkIdentity(category, key); err != nil {
		return CatalogEntry{}, err
	}
	v := c.Vault()
	if v == nil {
		return CatalogEntry{}, ErrVaultNotFound
	}

	slot, existing, err := c.find(category, key)
	if err != nil {
		return CatalogEntry{}, err
	}

	now := time.Now().UTC()
	env := NewEnvelope(category, key, value, description, now)
	if slot == "" {
		slot = NewSlot()
	} else {
		env.CreatedUTC = existing.CreatedUTC
	}

	raw, err := MarshalEnvelope(env)
	if err != nil {
		return CatalogEntry{}, err
	}
	if err := v.Set(slot, raw); err != nil {
		return CatalogEntry{}, err
	}
	return toCatalogEntry(slot, env), nil
}

// Get returns the value stored under (category, key).
func (c *Catalog) Get(category, key string) (string, error) {
	if err := checkIdentity(category, key); err != nil {
		return "", err
	}
	slot, env, err := c.find(category, key)
	if err != nil {
		return "", err
	}
	if slot == "" {
		return "", ErrNotFound
	}
	return env.Value, nil
}

// Has reports whether a secret exists under (category, key). Unlike the plain
// vault, this needs the vault unlocked: identity lives inside the ciphertext.
func (c *Catalog) Has(category, key string) bool {
	slot, _, err := c.find(category, key)
	return err == nil && slot != ""
}

// Delete removes the secret under (category, key).
func (c *Catalog) Delete(category, key string) error {
	if err := checkIdentity(category, key); err != nil {
		return err
	}
	v := c.Vault()
	if v == nil {
		return ErrVaultNotFound
	}
	slot, _, err := c.find(category, key)
	if err != nil {
		return err
	}
	if slot == "" {
		return ErrNotFound
	}
	return v.Delete(slot)
}

// List returns every secret's identity, sorted by category then key.
//
// Records that are not envelopes are skipped rather than failing the listing:
// another tool sharing this vault may store shapes this build predates, and
// one of those must not make the whole panel unreadable. Foreign records are
// still counted, so a caller can tell the user something is there.
func (c *Catalog) List() ([]CatalogEntry, error) {
	entries, _, err := c.listWithForeign()
	return entries, err
}

// ListWithForeign is List plus the number of records this schema could not
// read, so a UI can say "3 secrets, 1 record from another tool" instead of
// quietly under-reporting.
func (c *Catalog) ListWithForeign() ([]CatalogEntry, int, error) {
	return c.listWithForeign()
}

func (c *Catalog) listWithForeign() ([]CatalogEntry, int, error) {
	v := c.Vault()
	if v == nil {
		return nil, 0, ErrVaultNotFound
	}
	slots, err := v.List()
	if err != nil {
		return nil, 0, err
	}

	out := make([]CatalogEntry, 0, len(slots))
	foreign := 0
	for _, slot := range slots {
		if slot == reservedCatalogSlot {
			continue
		}
		raw, err := v.Get(slot)
		if err != nil {
			// A locked vault must fail the whole call, not be reported as a
			// pile of foreign records.
			if errors.Is(err, ErrLocked) {
				return nil, 0, err
			}
			foreign++
			continue
		}
		env, err := ParseEnvelope(raw)
		if err != nil {
			foreign++
			continue
		}
		out = append(out, toCatalogEntry(slot, env))
	}

	slices.SortFunc(out, func(a, b CatalogEntry) int {
		if n := cmp.Compare(a.Category, b.Category); n != 0 {
			return n
		}
		return cmp.Compare(a.Key, b.Key)
	})
	return out, foreign, nil
}

// find locates the slot holding (category, key). An empty slot with a nil
// error means "not stored", which is distinct from an error reading the vault.
// Matching is case-insensitive so "API-Client" and "api-client" are one thing.
func (c *Catalog) find(category, key string) (string, Envelope, error) {
	v := c.Vault()
	if v == nil {
		return "", Envelope{}, ErrVaultNotFound
	}
	slots, err := v.List()
	if err != nil {
		return "", Envelope{}, err
	}
	wantCat, wantKey := foldIdentity(category), foldIdentity(key)

	for _, slot := range slots {
		if slot == reservedCatalogSlot {
			continue
		}
		raw, err := v.Get(slot)
		if err != nil {
			if errors.Is(err, ErrLocked) {
				return "", Envelope{}, err
			}
			continue
		}
		env, err := ParseEnvelope(raw)
		if err != nil {
			continue
		}
		if foldIdentity(env.Category) == wantCat && foldIdentity(env.Key) == wantKey {
			return slot, env, nil
		}
	}
	return "", Envelope{}, nil
}

func toCatalogEntry(slot string, env Envelope) CatalogEntry {
	return CatalogEntry{
		Slot:        slot,
		Category:    env.Category,
		Key:         env.Key,
		Description: env.DescriptionOr(),
		CreatedUTC:  env.CreatedUTC.Time,
		UpdatedUTC:  env.UpdatedUTC.Time,
	}
}

// checkIdentity rejects the pairs the .NET reader would refuse, so this side
// can never write a record the other side treats as corrupt.
func checkIdentity(category, key string) error {
	if strings.TrimSpace(category) == "" || strings.TrimSpace(key) == "" {
		return ErrInvalidName
	}
	return nil
}

func foldIdentity(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
