package datacore

import "strings"

// idSep joins a record's template and file into one tensor identity, so the
// same filename living in two templates stays two distinct identities (the
// keystone for a tensor that spans more than one template). It is a control
// char (unit separator) that never appears in a template name or a filename,
// so a composite id is unambiguous to split and cheap to detect.
const idSep = "\x1f"

// NewID builds the tensor identity for a record from its template and id. An
// empty template yields the bare id, so callers that ingest standalone records
// (and the existing single-axis tests) keep their plain string identities.
func NewID(template, id string) string {
	if template == "" {
		return id
	}
	return template + idSep + id
}

// isCompositeID reports whether s already carries a template prefix, so the
// service can accept either a bare filename (first studio call) or a composite
// node id (click-to-unfold round-trip) without double-prefixing.
func isCompositeID(s string) bool { return strings.Contains(s, idSep) }
