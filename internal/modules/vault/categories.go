package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// The category list is itself a vault record, under a reserved slot, so
// category names are protected exactly like secret values. The shape is
// TaskBlaster's CategoryCatalog, so both tools read and write one list rather
// than each keeping its own.
const categoryCatalogSchemaVersion = 1

// ErrCategoryCatalogInvalid marks a reserved record that is not a catalog in
// this schema.
var ErrCategoryCatalogInvalid = errors.New("vault: not a valid category catalog")

// categoryCatalog is the persisted list. Field names are PascalCase to match
// the .NET writer.
type categoryCatalog struct {
	SchemaVersion int      `json:"SchemaVersion"`
	Categories    []string `json:"Categories"`
	UpdatedUTC    utcTime  `json:"UpdatedUtc"`
}

// NormalizeCategories trims, drops blanks, dedupes case-insensitively keeping
// the first-seen casing, and sorts case-insensitively. Same rules as the .NET
// side, so a list written by either tool round-trips unchanged.
func NormalizeCategories(names []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(names))
	for _, raw := range names {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		folded := strings.ToLower(trimmed)
		if seen[folded] {
			continue
		}
		seen[folded] = true
		out = append(out, trimmed)
	}
	slices.SortFunc(out, func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})
	return out
}

// Categories returns the category list the picker should offer: the persisted
// catalog merged with every category actually in use.
//
// Merging matters because the two can disagree. A category is in use the
// moment a secret carries it, whether or not anyone curated a list, and a
// curated name is worth offering before any secret uses it. Showing only one
// source would hide half the answer.
func (c *Catalog) Categories() ([]string, error) {
	v := c.Vault()
	if v == nil {
		return nil, ErrVaultNotFound
	}

	persisted, err := c.persistedCategories()
	if err != nil {
		return nil, err
	}
	entries, err := c.List()
	if err != nil {
		return nil, err
	}
	inUse := make([]string, 0, len(entries))
	for _, e := range entries {
		inUse = append(inUse, e.Category)
	}
	return NormalizeCategories(append(persisted, inUse...)), nil
}

// SetCategories replaces the persisted list. Categories still in use are kept
// regardless: dropping one would leave its secrets in a category the picker
// refuses to show, which reads as the secrets having vanished.
func (c *Catalog) SetCategories(names []string) ([]string, error) {
	v := c.Vault()
	if v == nil {
		return nil, ErrVaultNotFound
	}
	entries, err := c.List()
	if err != nil {
		return nil, err
	}
	keep := append([]string{}, names...)
	for _, e := range entries {
		keep = append(keep, e.Category)
	}

	final := NormalizeCategories(keep)
	raw, err := json.Marshal(categoryCatalog{
		SchemaVersion: categoryCatalogSchemaVersion,
		Categories:    final,
		UpdatedUTC:    utcTime{time.Now().UTC()},
	})
	if err != nil {
		return nil, err
	}
	if err := v.Set(reservedCatalogSlot, string(raw)); err != nil {
		return nil, err
	}
	return final, nil
}

// AddCategory records a new category name, so it is offered by the picker
// before any secret uses it.
func (c *Catalog) AddCategory(name string) ([]string, error) {
	if strings.TrimSpace(name) == "" {
		return nil, ErrInvalidName
	}
	current, err := c.Categories()
	if err != nil {
		return nil, err
	}
	return c.SetCategories(append(current, name))
}

// persistedCategories reads the reserved record. A missing one is an empty
// list, not an error: a vault that has never had its categories curated is
// perfectly normal. A malformed one is reported, because silently treating a
// tool's catalog as absent would let SetCategories overwrite it.
func (c *Catalog) persistedCategories() ([]string, error) {
	v := c.Vault()
	raw, err := v.Get(reservedCatalogSlot)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	var cat categoryCatalog
	if err := json.Unmarshal([]byte(raw), &cat); err != nil {
		return nil, fmt.Errorf("%w: malformed JSON", ErrCategoryCatalogInvalid)
	}
	if cat.SchemaVersion != categoryCatalogSchemaVersion {
		return nil, fmt.Errorf("%w: unsupported schema version %d", ErrCategoryCatalogInvalid, cat.SchemaVersion)
	}
	return NormalizeCategories(cat.Categories), nil
}
