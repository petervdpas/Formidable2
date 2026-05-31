package gigot

import (
	"encoding/json"
	"fmt"
)

// A record is the on-disk meta.json envelope: {"meta": {...}, "data": {...}}.
// Conflict resolution moves single field values between records (yours and the
// server's), so we work at the field level over these two scopes. The server
// canonicalizes record bytes on every merge, so re-marshalling here introduces
// no formatting churn beyond what a merge already does.

// getRecordField returns the raw JSON value of one field in the given scope
// ("data" or "meta"). ok is false when the scope or key is absent.
func getRecordField(record []byte, scope, key string) (json.RawMessage, bool, error) {
	var env map[string]json.RawMessage
	if err := json.Unmarshal(record, &env); err != nil {
		return nil, false, fmt.Errorf("gigot: parse record: %w", err)
	}
	raw, ok := env[scope]
	if !ok {
		return nil, false, nil
	}
	var section map[string]json.RawMessage
	if err := json.Unmarshal(raw, &section); err != nil {
		return nil, false, fmt.Errorf("gigot: parse %s: %w", scope, err)
	}
	v, ok := section[key]
	return v, ok, nil
}

// copyFields returns target with each named field replaced by source's value
// for that field. A field absent in source is left as-is in target. This is
// the primitive behind both neutralize-to-theirs and apply-mine.
func copyFields(target, source []byte, fields []FieldResolution) ([]byte, error) {
	out := target
	for _, f := range fields {
		v, ok, err := getRecordField(source, f.Scope, f.Key)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		out, err = setRecordField(out, f.Scope, f.Key, v)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// setRecordField writes value at scope/key and returns the re-marshalled
// record. A missing scope is created. value is stored verbatim (atomic), so a
// nested object replaces wholesale.
func setRecordField(record []byte, scope, key string, value json.RawMessage) ([]byte, error) {
	var env map[string]json.RawMessage
	if err := json.Unmarshal(record, &env); err != nil {
		return nil, fmt.Errorf("gigot: parse record: %w", err)
	}
	section := map[string]json.RawMessage{}
	if raw, ok := env[scope]; ok {
		if err := json.Unmarshal(raw, &section); err != nil {
			return nil, fmt.Errorf("gigot: parse %s: %w", scope, err)
		}
	}
	section[key] = value
	sectionBytes, err := json.Marshal(section)
	if err != nil {
		return nil, fmt.Errorf("gigot: marshal %s: %w", scope, err)
	}
	env[scope] = sectionBytes
	out, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("gigot: marshal record: %w", err)
	}
	return out, nil
}
