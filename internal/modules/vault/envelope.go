package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// The envelope layer stores each secret under an opaque 32-hex slot name, with
// the real category, key and value inside the encrypted blob. Nothing about a
// secret leaks from a directory listing: not which service it belongs to, not
// even how many of each kind there are.
//
// The schema is TaskBlaster's, byte for byte, so one vault directory serves
// both tools and each can read the other's records. That is why the JSON tags
// are PascalCase and why SchemaVersion, Category and Key are validated exactly
// as the .NET side validates them: a record either satisfies both readers or
// neither.
const (
	envelopeSchemaVersion = 1

	// reservedCatalogSlot holds the category list. It is not a secret and must
	// never be listed as one.
	reservedCatalogSlot = "00000000000000000000000000000001"
)

// ErrEnvelopeInvalid marks a record that is readable but not a secret in this
// schema: a foreign format, or a newer version this build predates.
var ErrEnvelopeInvalid = errors.New("vault: not a valid secret envelope")

// Envelope is the decrypted payload of one slot. Description is a pointer
// because the .NET writer emits an explicit null rather than omitting it.
type Envelope struct {
	SchemaVersion int     `json:"SchemaVersion"`
	Category      string  `json:"Category"`
	Key           string  `json:"Key"`
	Value         string  `json:"Value"`
	Description   *string `json:"Description"`
	CreatedUTC    utcTime `json:"CreatedUtc"`
	UpdatedUTC    utcTime `json:"UpdatedUtc"`
}

// NewEnvelope builds a fresh envelope, trimming the identity fields the same
// way the .NET side does so a round trip is stable.
func NewEnvelope(category, key, value, description string, now time.Time) Envelope {
	e := Envelope{
		SchemaVersion: envelopeSchemaVersion,
		Category:      strings.TrimSpace(category),
		Key:           strings.TrimSpace(key),
		Value:         value,
		CreatedUTC:    utcTime{now.UTC()},
		UpdatedUTC:    utcTime{now.UTC()},
	}
	e.SetDescription(description)
	return e
}

// SetDescription stores a description, collapsing blank to absent so the two
// implementations agree on what "no description" looks like.
func (e *Envelope) SetDescription(description string) {
	if strings.TrimSpace(description) == "" {
		e.Description = nil
		return
	}
	d := description
	e.Description = &d
}

// DescriptionOr returns the description, or the empty string when absent.
func (e Envelope) DescriptionOr() string {
	if e.Description == nil {
		return ""
	}
	return *e.Description
}

// MarshalEnvelope serialises without indentation, matching the .NET writer.
func MarshalEnvelope(e Envelope) (string, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ParseEnvelope decodes and validates a slot payload. The checks mirror the
// .NET reader exactly: anything it would reject is rejected here, so a record
// never looks valid to one tool and corrupt to the other.
func ParseEnvelope(raw string) (Envelope, error) {
	if strings.TrimSpace(raw) == "" {
		return Envelope{}, fmt.Errorf("%w: empty", ErrEnvelopeInvalid)
	}
	var e Envelope
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		return Envelope{}, fmt.Errorf("%w: malformed JSON", ErrEnvelopeInvalid)
	}
	if e.SchemaVersion != envelopeSchemaVersion {
		return Envelope{}, fmt.Errorf("%w: unsupported schema version %d", ErrEnvelopeInvalid, e.SchemaVersion)
	}
	if strings.TrimSpace(e.Category) == "" {
		return Envelope{}, fmt.Errorf("%w: empty category", ErrEnvelopeInvalid)
	}
	if strings.TrimSpace(e.Key) == "" {
		return Envelope{}, fmt.Errorf("%w: empty key", ErrEnvelopeInvalid)
	}
	return e, nil
}

// NewSlot mints an opaque slot name: a 32-char lowercase hex GUID, the same
// shape .NET's Guid.ToString("N") produces, so filenames are uniform and leak
// nothing about what they hold.
func NewSlot() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}
