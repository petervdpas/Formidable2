package query

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Saved queries live in one of two roots, chosen per profile by the
// saved_query_location setting. One file per saved query, filed under the
// template it targets, so listing a template's queries is a directory read:
//
//	local:  <AppRoot>/queries/<template-stem>/<id>.json
//	shared: <storage>/<template-stem>/queries/<id>.json
//
// Local is the default and stays on this machine: it sits outside the context
// folder, so saving a query never produces a commit. Shared puts the file in
// the storage tree, which git and gigot already sync recursively, for a query
// the whole team should have. Records are matched by their .meta.json
// extension, so the shared queries/ subfolder is inert to the form readers.
const (
	savedExt   = ".json"
	dirQueries = "queries"
)

// Saved-query locations. The set is closed and backend-owned; the settings
// picker renders it rather than restating it.
const (
	LocationLocal  = "local"
	LocationShared = "shared"
)

// StoreLocations is the closed location set, in display order.
var StoreLocations = []string{LocationLocal, LocationShared}

// NormalizeLocation maps anything unrecognised (an empty setting, a hand-edited
// user.json, a value from a newer version) to LocationLocal. Local is the root
// that never touches a synced tree, so a setting we cannot read can never
// publish a query by accident.
func NormalizeLocation(v string) string {
	if strings.TrimSpace(v) == LocationShared {
		return LocationShared
	}
	return LocationLocal
}

// ErrQueryNotFound is returned when no saved query is stored under an id.
var ErrQueryNotFound = errors.New("query: saved query not found")

// SavedQuery is a named Spec. The Spec is stored verbatim, so what reopens is
// exactly what the engine ran: the builder UI is reconstructed from it rather
// than persisted alongside it. Location is where the record was found, filled
// in on read and never written to the file: the folder it sits in is the one
// source of truth for that.
type SavedQuery struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Template string `json:"template"`
	Location string `json:"location,omitempty"`
	Spec     Spec   `json:"spec"`
	Created  string `json:"created,omitempty"`
	Updated  string `json:"updated,omitempty"`
}

// storeFS is the filesystem surface the saved-query store needs. SaveFile is
// expected to be atomic and to create missing parents.
type storeFS interface {
	LoadFile(path string) (string, error)
	SaveFile(path, content string) error
	DeleteFile(path string) error
	ListDir(path string) ([]string, error)
}

// Store owns the saved queries on disk. Writes are serialized so a
// read-modify-write (Created preservation) cannot interleave with another.
type Store struct {
	fs         storeFS
	localRoot  string
	sharedRoot string
	defaultLoc func() string
	log        *slog.Logger

	mu  sync.Mutex
	now func() time.Time
}

// NewStore builds a store over both roots (absolute, resolved by the
// composition root). defaultLocation reads the profile setting that decides
// where a new save lands; nil pins it to local. log may be nil.
func NewStore(fs storeFS, localRoot, sharedRoot string, defaultLocation func() string, log *slog.Logger) *Store {
	if log == nil {
		log = slog.Default()
	}
	if localRoot == "" {
		localRoot = dirQueries
	}
	if sharedRoot == "" {
		sharedRoot = "storage"
	}
	if defaultLocation == nil {
		defaultLocation = func() string { return LocationLocal }
	}
	return &Store{
		fs:         fs,
		localRoot:  localRoot,
		sharedRoot: sharedRoot,
		defaultLoc: defaultLocation,
		log:        log,
		now:        time.Now,
	}
}

// DefaultLocation is where a save with no location of its own lands: the
// profile setting, normalized.
func (s *Store) DefaultLocation() string { return NormalizeLocation(s.defaultLoc()) }

// List returns every saved query for a template from both locations, tagged
// with where it lives and sorted by name. Both are read whatever the setting
// says: the setting picks where new saves land, it never hides what is already
// there, and a query a peer shared must stay visible to someone saving locally.
// A file that fails to parse is logged and skipped: it cannot be opened, so
// listing it would only offer the user a dead entry.
func (s *Store) List(templateFilename string) ([]SavedQuery, error) {
	stem, err := templateStem(templateFilename)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fs == nil {
		return nil, errors.New("query: no filesystem")
	}

	out := []SavedQuery{}
	for _, loc := range StoreLocations {
		entries, err := s.fs.ListDir(s.dirFor(stem, loc))
		if err != nil {
			return nil, fmt.Errorf("query: cannot list saved queries: %w", err)
		}
		for _, e := range entries {
			if !strings.HasSuffix(e, savedExt) {
				continue
			}
			id := strings.TrimSuffix(e, savedExt)
			q, err := s.load(stem, id, loc)
			if err != nil {
				s.log.Warn("query: skipping unreadable saved query", "template", templateFilename, "location", loc, "id", id, "err", err)
				continue
			}
			out = append(out, q)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if a != b {
			return a < b
		}
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Location < out[j].Location
	})
	return out, nil
}

// Get returns one saved query from one location.
func (s *Store) Get(templateFilename, id, location string) (SavedQuery, error) {
	stem, err := templateStem(templateFilename)
	if err != nil {
		return SavedQuery{}, err
	}
	if err := checkQuerySlug(id); err != nil {
		return SavedQuery{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fs == nil {
		return SavedQuery{}, errors.New("query: no filesystem")
	}
	return s.load(stem, id, NormalizeLocation(location))
}

// Save validates and writes a saved query, returning the stored record (id
// filled in, location resolved, timestamps stamped). An empty ID is derived
// from the name, so the caller can save by name alone. An empty Location takes
// the profile default, so re-saving a query keeps it where it already lives
// instead of quietly moving it into (or out of) the synced tree.
func (s *Store) Save(q SavedQuery) (SavedQuery, error) {
	stem, err := templateStem(q.Template)
	if err != nil {
		return SavedQuery{}, err
	}
	q.Name = strings.TrimSpace(q.Name)
	if q.Name == "" {
		return SavedQuery{}, errors.New("query: name required")
	}
	if q.ID == "" {
		q.ID = slugify(q.Name)
	}
	if err := checkQuerySlug(q.ID); err != nil {
		return SavedQuery{}, err
	}
	if len(q.Spec.Columns) == 0 {
		return SavedQuery{}, errors.New("query: at least one column required")
	}
	// The spec is what runs; it must target the template it is filed under.
	q.Spec.Template = q.Template

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fs == nil {
		return SavedQuery{}, errors.New("query: no filesystem")
	}
	if q.Location == "" {
		q.Location = s.DefaultLocation()
	}
	q.Location = NormalizeLocation(q.Location)

	stamp := s.now().UTC().Format(time.RFC3339Nano)
	if prev, err := s.load(stem, q.ID, q.Location); err == nil && prev.Created != "" {
		q.Created = prev.Created
	} else if q.Created == "" {
		q.Created = stamp
	}
	q.Updated = stamp

	// Location is positional: the folder the file sits in already says it, and
	// writing it would let a synced copy contradict where it landed.
	onDisk := q
	onDisk.Location = ""
	out, err := json.MarshalIndent(onDisk, "", "  ")
	if err != nil {
		return SavedQuery{}, fmt.Errorf("query: cannot serialize %q: %w", q.ID, err)
	}
	if err := s.fs.SaveFile(s.path(stem, q.ID, q.Location), string(out)+"\n"); err != nil {
		return SavedQuery{}, fmt.Errorf("query: cannot store %q: %w", q.ID, err)
	}
	return q, nil
}

// Delete removes a saved query from one location. Deleting one that is already
// gone succeeds.
func (s *Store) Delete(templateFilename, id, location string) error {
	stem, err := templateStem(templateFilename)
	if err != nil {
		return err
	}
	if err := checkQuerySlug(id); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fs == nil {
		return errors.New("query: no filesystem")
	}
	loc := NormalizeLocation(location)
	if _, err := s.load(stem, id, loc); errors.Is(err, ErrQueryNotFound) {
		return nil
	}
	return s.fs.DeleteFile(s.path(stem, id, loc))
}

// dirFor is the folder a template's saved queries live in for one location.
func (s *Store) dirFor(stem, location string) string {
	if NormalizeLocation(location) == LocationShared {
		return filepath.Join(s.sharedRoot, stem, dirQueries)
	}
	return filepath.Join(s.localRoot, stem)
}

func (s *Store) path(stem, id, location string) string {
	return filepath.Join(s.dirFor(stem, location), id+savedExt)
}

// load reads one record. Callers hold the mutex.
func (s *Store) load(stem, id, location string) (SavedQuery, error) {
	raw, err := s.fs.LoadFile(s.path(stem, id, location))
	if err != nil {
		return SavedQuery{}, ErrQueryNotFound
	}
	var q SavedQuery
	if err := json.Unmarshal([]byte(raw), &q); err != nil {
		return SavedQuery{}, fmt.Errorf("query: cannot parse saved query %q: %w", id, err)
	}
	q.ID = id
	q.Location = NormalizeLocation(location)
	return q, nil
}

// templateStem validates a template filename and returns its stem, the folder
// saved queries for it live under. Separators and traversal are refused rather
// than sanitized away: a path-shaped template name is a caller bug.
func templateStem(templateFilename string) (string, error) {
	name := strings.TrimSpace(templateFilename)
	if name == "" {
		return "", errors.New("query: template required")
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", fmt.Errorf("query: invalid template name %q", templateFilename)
	}
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	if stem == "" {
		return "", fmt.Errorf("query: invalid template name %q", templateFilename)
	}
	return stem, nil
}

// checkQuerySlug refuses anything that is not a bare lowercase identifier, so
// an id can never reach outside its template's folder.
func checkQuerySlug(id string) error {
	if id == "" {
		return errors.New("query: id required")
	}
	if len(id) > 80 {
		return fmt.Errorf("query: id too long: %q", id)
	}
	for i, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case (r == '-' || r == '_') && i > 0:
		default:
			return fmt.Errorf("query: invalid id %q", id)
		}
	}
	return nil
}

// slugify derives an id from a display name: lowercase, runs of anything else
// collapsed to a single dash.
func slugify(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if len(out) > 80 {
		out = strings.TrimRight(out[:80], "-")
	}
	return out
}
