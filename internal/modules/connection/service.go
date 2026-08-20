package connection

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"
)

// SecretCategory namespaces api-client credentials inside a shared secret
// store, so a client named "northwind" cannot collide with a git remote of the
// same name in the same vault.
const SecretCategory = "api-client"

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

	// docsPorts are ports the docs server must not bind, typically the
	// configured internal server port.
	docsPorts []int

	// The docs server binds a port, and most sessions never open the Document
	// tab, so it starts on first use rather than at boot.
	docsMu sync.Mutex
	docs   *DocsServer
}

// NewService binds a Manager and an Invoker for Wails. reservedPorts are ports
// the lazily-started docs server must avoid.
func NewService(m *Manager, inv *Invoker, keys SecretWriter, reservedPorts ...int) *Service {
	return &Service{m: m, inv: inv, keys: keys, docsPorts: reservedPorts}
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
	cat, err := s.catalogFor(c)
	if err != nil {
		return nil, err
	}
	errs := Validate(&c, cat)
	if errs == nil {
		errs = []ValidationError{}
	}
	return errs, nil
}

// ListOperations returns every operation the client's document declares, with
// what its response reads as and which resources already bind it. The draft
// goes over rather than an id so the bound-by column tracks unsaved edits.
func (s *Service) ListOperations(c Connection) ([]OperationInfo, error) {
	cat, err := s.catalogFor(c)
	if err != nil {
		return nil, err
	}
	return Operations(cat, &c), nil
}

// ListMethods returns the HTTP methods a catalog can contain with the colours
// their badges render in, so the operation list needs no palette of its own.
func (s *Service) ListMethods() []MethodDescriptor { return Methods() }

// ListShapes returns the response shapes an operation row can carry, for the
// editor's filter.
func (s *Service) ListShapes() []string {
	return []string{ShapeRecords, ShapeKeyed, ShapeValues, ShapeKeyedValues, ShapeRecord, ShapeUnknown}
}

// DetectResources proposes resources by reading the client's spec, for the
// detect button in the resource editor. It takes the draft rather than an id so
// an unsaved edit counts: whatever the draft already binds is left out of the
// proposal, and its keys are not reused.
func (s *Service) DetectResources(c Connection) (Detection, error) {
	cat, err := s.catalogFor(c)
	if err != nil {
		return Detection{}, err
	}
	return Detect(cat, &c), nil
}

// catalogFor resolves the spec behind a draft client. An unsaved client has no
// stored definition to read a spec through, so the spec file it names is the
// fallback.
func (s *Service) catalogFor(c Connection) (*Catalog, error) {
	cat, err := s.m.Catalog(c.ID)
	if err == nil {
		return cat, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return s.m.CatalogForSpec(c.SpecFile)
}

// SpecSource returns the uploaded document behind a client, for the Document
// tab. The draft goes over rather than an id so a client whose upload is not
// saved yet still shows what was just imported.
func (s *Service) SpecSource(c Connection) (SpecSource, error) {
	return s.m.SpecSource(c.SpecFile)
}

// SpecDocument returns the uploaded document as JSON, for rendering it with
// Swagger UI.
func (s *Service) SpecDocument(c Connection) (SpecDocument, error) {
	return s.m.SpecDocument(c.SpecFile)
}

// DocsURL returns the page that renders a client's document with Swagger UI,
// starting the loopback server that serves it on first use.
func (s *Service) DocsURL(c Connection) (string, error) {
	if err := checkSpecFile(c.SpecFile); err != nil {
		return "", err
	}
	// Fail before binding a port when the document cannot be read at all.
	if _, err := s.m.SpecDocument(c.SpecFile); err != nil {
		return "", err
	}

	s.docsMu.Lock()
	defer s.docsMu.Unlock()
	if s.docs == nil || s.docs.Addr() == "" {
		d, err := NewDocsServer(s.m, nil, s.docsPorts...)
		if err != nil {
			return "", err
		}
		s.docs = d
	}
	return s.docs.URLFor(c.SpecFile), nil
}

// CloseDocs stops the docs server. Wired into app shutdown; safe to call when
// nothing ever started it.
func (s *Service) CloseDocs() error {
	s.docsMu.Lock()
	defer s.docsMu.Unlock()
	err := s.docs.Close()
	s.docs = nil
	return err
}

// ReloadSpecs drops the parsed-spec cache, for after a spec file is edited by
// hand outside the app.
func (s *Service) ReloadSpecs() { s.m.Reload() }

// ListDialects returns the known protocol presets for the editor's picker.
func (s *Service) ListDialects() []string { return KnownDialects() }

// ListKeyStyles returns the known entity-addressing strategies.
func (s *Service) ListKeyStyles() []string { return []string{KeyRaw, KeyQuoted, KeyTyped} }

// ListItemsModes returns the known item-container shapes, for the picker in the
// resource editor.
func (s *Service) ListItemsModes() []string { return []string{ItemsArray, ItemsMap} }

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

// FetchSnapshot fetches one remote record and returns it in the shape the
// api-client field stores: Select names the fields to keep. An error yields no
// snapshot, so a caller keeps the copy already on disk instead of blanking it.
func (s *Service) FetchSnapshot(req FetchRequest) (map[string]any, error) {
	item, err := s.FetchItem(req)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, invokeErr(CodeRemoteNotFound, "no record for id "+req.ID, nil)
	}
	return Snapshot(*item, req.Select, time.Now()), nil
}

// TryOperationForm describes what running an operation would need: its
// parameters, whatever a resource already fixes for them, and whether the
// console will run it at all.
func (s *Service) TryOperationForm(c Connection, operation string) (TryForm, error) {
	cat, err := s.catalogFor(c)
	if err != nil {
		return TryForm{}, err
	}
	return BuildTryForm(cat, &c, operation)
}

// TryOperation runs one operation straight from the document and returns what
// came back, including a failing status: seeing the remote's own error is the
// point of a console.
func (s *Service) TryOperation(req TryRequest) (*TryResult, error) {
	if s.inv == nil {
		return nil, invokeErr(CodeConnectionNotFound, "no invoker configured", nil)
	}
	return s.inv.Try(context.Background(), req)
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
