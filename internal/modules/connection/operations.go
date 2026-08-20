package connection

// The editor lists every operation the document declares, not just the ones a
// resource binds. Reading a spec in the app is otherwise impossible: the
// pickers show operation ids, and nothing says what any of them returns or
// which are already spoken for.

// Response shapes an operation can have, as the editor labels them. The
// distinctions are the ones that decide whether an operation can back a
// resource at all.
const (
	// ShapeUnknown: the document declares no JSON response body. Common, and
	// not an error: plenty of specs describe only their inputs.
	ShapeUnknown = "unknown"

	// ShapeRecord: one object. A get candidate, never a list.
	ShapeRecord = "record"

	// ShapeRecords: an array of objects.
	ShapeRecords = "records"

	// ShapeValues: an array of plain values, each its own id.
	ShapeValues = "values"

	// ShapeKeyed: an object keyed by id, values are records.
	ShapeKeyed = "keyed"

	// ShapeKeyedValues: an object keyed by id, values are plain.
	ShapeKeyedValues = "keyed_values"
)

// Binding roles a resource can put an operation in.
const (
	RoleList = "list"
	RoleGet  = "get"
)

// OperationBinding names a resource that already uses an operation.
type OperationBinding struct {
	Resource string `json:"resource"`
	Role     string `json:"role"`
}

// OperationInfo is one catalog operation with what the editor needs to make
// sense of it: what its response reads as, and who already binds it.
type OperationInfo struct {
	Operation Operation          `json:"operation"`
	Shape     string             `json:"shape"`
	BoundBy   []OperationBinding `json:"bound_by,omitempty"`

	// Collection says the response yields items, so the operation could back a
	// resource's list binding. It is the single fact that decides whether
	// detection would ever propose it.
	Collection bool `json:"collection"`
}

// Operations annotates the catalog against a connection. A nil connection
// means nothing is bound yet, which is what a freshly uploaded document is.
func Operations(cat *Catalog, c *Connection) []OperationInfo {
	if cat == nil {
		return []OperationInfo{}
	}

	bound := map[string][]OperationBinding{}
	if c != nil {
		for _, r := range c.Resources {
			if r.List.Operation != "" {
				bound[r.List.Operation] = append(bound[r.List.Operation],
					OperationBinding{Resource: r.Key, Role: RoleList})
			}
			if r.Get.Operation != "" {
				bound[r.Get.Operation] = append(bound[r.Get.Operation],
					OperationBinding{Resource: r.Key, Role: RoleGet})
			}
		}
	}

	out := make([]OperationInfo, 0, len(cat.Operations))
	for _, op := range cat.Operations {
		out = append(out, OperationInfo{
			Operation:  op,
			Shape:      shapeName(op.Result),
			BoundBy:    bound[op.ID],
			Collection: op.Result != nil && op.Result.Collection,
		})
	}
	return out
}

// shapeName reduces a distilled response to the one word the editor shows.
func shapeName(res *Result) string {
	switch {
	case res == nil:
		return ShapeUnknown
	case !res.Collection:
		return ShapeRecord
	case res.ItemsMode == ItemsMap && res.Scalar:
		return ShapeKeyedValues
	case res.ItemsMode == ItemsMap:
		return ShapeKeyed
	case res.Scalar:
		return ShapeValues
	default:
		return ShapeRecords
	}
}
