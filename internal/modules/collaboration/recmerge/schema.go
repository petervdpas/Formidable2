package recmerge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// Record is the parsed form of a *.meta.json file; Meta and Data are generic maps (the merger treats each value as atomic).
type Record struct {
	Meta map[string]any `json:"meta"`
	Data map[string]any `json:"data"`
}

// ParseRecord decodes raw JSON into a Record; missing meta/data become empty maps, empty input returns an empty Record.
func ParseRecord(raw []byte) (Record, error) {
	rec := Record{
		Meta: map[string]any{},
		Data: map[string]any{},
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return rec, nil
	}

	var envelope struct {
		Meta map[string]any `json:"meta"`
		Data map[string]any `json:"data"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&envelope); err != nil {
		return Record{}, fmt.Errorf("%w: %v", ErrMalformedRecord, err)
	}
	if dec.More() {
		return Record{}, fmt.Errorf("%w: trailing data after record", ErrMalformedRecord)
	}
	if envelope.Meta != nil {
		rec.Meta = envelope.Meta
	}
	if envelope.Data != nil {
		rec.Data = envelope.Data
	}
	return rec, nil
}

// CanonicalJSON re-serialises with map keys sorted at every depth (array order preserved).
// Byte-identical output across servers is load-bearing for mirror-sync and stable commit hashes.
func (r Record) CanonicalJSON() ([]byte, error) {
	envelope := map[string]any{
		"meta": nilToEmpty(r.Meta),
		"data": nilToEmpty(r.Data),
	}
	var buf bytes.Buffer
	if err := writeCanonical(&buf, envelope); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func nilToEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func writeCanonical(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case nil:
		buf.WriteString("null")
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return err
			}
			buf.Write(kb)
			buf.WriteByte(':')
			if err := writeCanonical(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, el := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonical(buf, el); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		buf.Write(b)
	}
	return nil
}
