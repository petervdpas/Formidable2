package connection

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
)

// SecretPrefix namespaces api-client credentials inside a shared secret store,
// so a client named "northwind" cannot collide with a git remote of the same
// name sitting in the same vault.
const SecretPrefix = "api-client-"

// SecretWriter is the credential side of a secret store. The invoker only
// reads; the API Clients panel needs to write and forget too, without either
// side knowing the store is a vault.
type SecretWriter interface {
	Store(id, secret string) error
	Forget(id string) error
	Has(id string) bool
}

// Service is the Wails-bound facade for API Clients.
//
// Credentials move one way: SetCredential takes a value in, and nothing hands
// one back out. HasCredential answers the only question the panel actually has,
// which is whether a client is configured.
type Service struct {
	m    *Manager
	inv  *Invoker
	keys SecretWriter
}

// NewService binds a Manager and an Invoker for Wails.
func NewService(m *Manager, inv *Invoker, keys SecretWriter) *Service {
	return &Service{m: m, inv: inv, keys: keys}
}

// ClientDetail is one client plus everything the editor needs to bind it: the
// operations its spec offers, and whether a credential is on file.
type ClientDetail struct {
	Client        Connection `json:"client"`
	Catalog       *Catalog   `json:"catalog"`
	HasCredential bool       `json:"has_credential"`
}

// ListClients returns every stored client, sorted, with a per-row OK flag.
func (s *Service) ListClients() ([]Summary, error) { return s.m.List() }

// GetClient returns one client with its parsed spec.
func (s *Service) GetClient(id string) (*ClientDetail, error) {
	c, cat, err := s.m.Get(id)
	if err != nil {
		return nil, err
	}
	return &ClientDetail{Client: *c, Catalog: cat, HasCredential: s.hasCredential(id)}, nil
}

// SaveClient validates every binding against the spec and writes only if all
// of them hold, so a stored client is always executable.
func (s *Service) SaveClient(c Connection) error { return s.m.Save(&c) }

// DeleteClient removes a client, its spec when nothing else uses it, and its
// stored credential.
func (s *Service) DeleteClient(id string) error {
	if err := s.m.Delete(id); err != nil {
		return err
	}
	if s.keys != nil {
		// A leftover credential for a client that no longer exists is a secret
		// nobody will ever look at again. Not fatal if the store is locked.
		_ = s.keys.Forget(id)
	}
	return nil
}

// ImportSpec stores an uploaded Swagger or OpenAPI document under
// api-clients/specs/ and returns the parsed catalog. The body is base64 (or a
// data URI), matching how the fonts and cover uploads cross the boundary.
func (s *Service) ImportSpec(name, base64Data string) (SpecInfo, error) {
	raw, err := decodeBody(base64Data)
	if err != nil {
		return SpecInfo{}, err
	}
	return s.m.ImportSpec(name, raw)
}

// GetCatalog returns the operations a client's spec offers, for the operation
// pickers in the resource editor.
func (s *Service) GetCatalog(id string) (*Catalog, error) { return s.m.Catalog(id) }

// ValidateClient reports what is wrong with a client without saving it, so the
// editor can show problems while the user is still typing.
func (s *Service) ValidateClient(c Connection) ([]ValidationError, error) {
	cat, err := s.m.Catalog(c.ID)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		// An unsaved client has no stored definition to read a spec through;
		// fall back to the spec file the draft names.
		cat, err = s.m.CatalogForSpec(c.SpecFile)
		if err != nil {
			return nil, err
		}
	}
	errs := Validate(&c, cat)
	if errs == nil {
		errs = []ValidationError{}
	}
	return errs, nil
}

// ReloadSpecs drops the parsed-spec cache, for after a spec file is edited by
// hand outside the app.
func (s *Service) ReloadSpecs() { s.m.Reload() }

// ListDialects returns the known protocol presets for the editor's picker.
func (s *Service) ListDialects() []string { return KnownDialects() }

// ListKeyStyles returns the known entity-addressing strategies.
func (s *Service) ListKeyStyles() []string { return []string{KeyRaw, KeyQuoted, KeyTyped} }

// ListPaginationStyles returns the known paging strategies.
func (s *Service) ListPaginationStyles() []string {
	return []string{PageNone, PageOffset, PagePage, PageCursor, PageLink}
}

// ListItems runs a client's list binding. This is what a reference field's
// picker calls, and what the editor's "try it" button calls.
func (s *Service) ListItems(req ListRequest) (*Page, error) {
	if s.inv == nil {
		return nil, invokeErr(CodeConnectionNotFound, "no invoker configured", nil)
	}
	return s.inv.List(context.Background(), req)
}

// FetchItem resolves one stored id back to a record.
func (s *Service) FetchItem(req FetchRequest) (*Item, error) {
	if s.inv == nil {
		return nil, invokeErr(CodeConnectionNotFound, "no invoker configured", nil)
	}
	return s.inv.Fetch(context.Background(), req)
}

// SetCredential stores the secret a client authenticates with.
func (s *Service) SetCredential(id, secret string) error {
	if s.keys == nil {
		return invokeErr(CodeNotConfigured, "no secret store configured", nil)
	}
	return s.keys.Store(id, secret)
}

// ForgetCredential removes a client's stored secret.
func (s *Service) ForgetCredential(id string) error {
	if s.keys == nil {
		return invokeErr(CodeNotConfigured, "no secret store configured", nil)
	}
	return s.keys.Forget(id)
}

// HasCredential reports whether a secret is on file for a client. It does not
// need the store unlocked, so the panel can show configured state before
// prompting for a master password.
func (s *Service) HasCredential(id string) bool { return s.hasCredential(id) }

func (s *Service) hasCredential(id string) bool {
	return s.keys != nil && s.keys.Has(id)
}

// decodeBody accepts a bare base64 payload or a data URI, so the frontend can
// hand over a FileReader result unchanged.
func decodeBody(body string) ([]byte, error) {
	body = strings.TrimSpace(body)
	if _, after, found := strings.Cut(body, ";base64,"); found {
		body = after
	}
	raw, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		return nil, errors.New("connection: spec upload is not valid base64")
	}
	return raw, nil
}
