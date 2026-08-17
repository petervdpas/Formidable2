package connection

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// slugRe is the identity rule for connection ids and resource keys. Both end up
// in filenames and in field attributes, so they stay lowercase, ASCII, and free
// of separators that could walk out of the connections directory.
var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// Validate checks a connection against the catalog parsed from its spec and
// returns every problem it finds, rather than stopping at the first. An empty
// slice means the interpreter can execute every binding without guessing.
func Validate(c *Connection, cat *Catalog) []ValidationError {
	if c == nil {
		return []ValidationError{{Type: "invalid-connection", Message: "Missing connection"}}
	}
	if cat == nil {
		return []ValidationError{{Type: "missing-catalog", Message: "Connection has no parsed spec"}}
	}

	d := dialectOf(c)

	var errs []ValidationError
	errs = append(errs, validateIdentity(c)...)
	errs = append(errs, validateBaseURL(c, cat)...)
	errs = append(errs, validateAuth(c.Auth)...)

	seen := map[string]bool{}
	for _, r := range c.Resources {
		if !slugRe.MatchString(r.Key) {
			errs = append(errs, ValidationError{
				Type:     "invalid-resource-key",
				Resource: r.Key,
				Message:  fmt.Sprintf("Resource key %q must be lowercase letters, digits, dot, dash or underscore", r.Key),
			})
			continue
		}
		if seen[r.Key] {
			errs = append(errs, ValidationError{
				Type:     "duplicate-resource-key",
				Resource: r.Key,
				Message:  fmt.Sprintf("Resource key %q is used more than once", r.Key),
			})
			continue
		}
		seen[r.Key] = true
		errs = append(errs, validateResource(r, cat, d)...)
	}
	return errs
}

func validateIdentity(c *Connection) []ValidationError {
	var errs []ValidationError
	if !slugRe.MatchString(c.ID) {
		errs = append(errs, ValidationError{
			Type:    "invalid-id",
			Message: fmt.Sprintf("Connection id %q must be lowercase letters, digits, dot, dash or underscore", c.ID),
		})
	}
	if strings.TrimSpace(c.Name) == "" {
		errs = append(errs, ValidationError{Type: "missing-name", Message: "Connection needs a name"})
	}
	if strings.TrimSpace(c.SpecFile) == "" {
		errs = append(errs, ValidationError{Type: "missing-spec", Message: "Connection needs a spec file"})
	}
	if !validDialect(c.Dialect) {
		errs = append(errs, ValidationError{
			Type:    "invalid-dialect",
			Message: fmt.Sprintf("Unknown dialect %q; known: %s", c.Dialect, strings.Join(KnownDialects(), ", ")),
			Detail:  map[string]any{"dialect": c.Dialect},
		})
	}
	return errs
}

// validateBaseURL accepts an empty BaseURL only when the spec supplies a server
// to fall back to, so a connection can never end up with nowhere to call.
func validateBaseURL(c *Connection, cat *Catalog) []ValidationError {
	if strings.TrimSpace(c.BaseURL) == "" {
		if len(cat.Servers) == 0 {
			return []ValidationError{{
				Type:    "no-server",
				Message: "No base URL set and the spec declares no server",
			}}
		}
		return nil
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || !u.IsAbs() || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return []ValidationError{{
			Type:    "invalid-base-url",
			Message: fmt.Sprintf("Base URL %q must be an absolute http or https URL", c.BaseURL),
			Detail:  map[string]any{"base_url": c.BaseURL},
		}}
	}
	return nil
}

func validateAuth(a Auth) []ValidationError {
	switch a.Kind {
	case "", AuthNone, AuthBearer:
		return nil
	case AuthAPIKey:
		if strings.TrimSpace(a.Name) == "" || (a.In != InHeader && a.In != InQuery) {
			return []ValidationError{{
				Type:    "incomplete-auth",
				Message: "API key auth needs a name and an `in` of header or query",
			}}
		}
		return nil
	case AuthBasic:
		if strings.TrimSpace(a.User) == "" {
			return []ValidationError{{Type: "incomplete-auth", Message: "Basic auth needs a user name"}}
		}
		return nil
	default:
		return []ValidationError{{
			Type:    "invalid-auth-kind",
			Message: fmt.Sprintf("Unknown auth kind %q", a.Kind),
			Detail:  map[string]any{"kind": a.Kind},
		}}
	}
}

func validateResource(r Resource, cat *Catalog, d dialectSpec) []ValidationError {
	var errs []ValidationError
	s := strategyFor(nil, r)
	s.linkPath = firstNonEmpty(r.Pagination.LinkPath, d.linkPath)

	errs = append(errs, validateStrategy(r, s)...)

	pointers := []struct{ label, ptr string }{
		{"items_path", r.ItemsPath},
		{"id_path", r.IDPath},
		{"label_path", r.LabelPath},
	}
	for _, p := range pointers {
		if !ValidPointer(p.ptr) {
			errs = append(errs, ValidationError{
				Type:     "invalid-pointer",
				Resource: r.Key,
				Message:  fmt.Sprintf("%s %q is not a valid JSON pointer", p.label, p.ptr),
				Detail:   map[string]any{"field": p.label, "pointer": p.ptr},
			})
		}
	}

	seenField := map[string]bool{}
	for _, f := range r.Fields {
		switch {
		case !slugRe.MatchString(f.Key):
			errs = append(errs, ValidationError{
				Type:     "invalid-field-key",
				Resource: r.Key,
				Message:  fmt.Sprintf("Field key %q must be lowercase letters, digits, dot, dash or underscore", f.Key),
			})
			continue
		case seenField[f.Key]:
			errs = append(errs, ValidationError{
				Type:     "duplicate-field-key",
				Resource: r.Key,
				Message:  fmt.Sprintf("Field key %q is declared more than once", f.Key),
			})
			continue
		}
		seenField[f.Key] = true
		if !ValidPointer(f.Pointer) {
			errs = append(errs, ValidationError{
				Type:     "invalid-pointer",
				Resource: r.Key,
				Message:  fmt.Sprintf("Field %q pointer %q is not a valid JSON pointer", f.Key, f.Pointer),
				Detail:   map[string]any{"field": f.Key, "pointer": f.Pointer},
			})
		}
	}

	listOp, ok := cat.Op(r.List.Operation)
	if !ok {
		errs = append(errs, ValidationError{
			Type:     "unknown-operation",
			Resource: r.Key,
			Message:  fmt.Sprintf("List operation %q is not in the spec", r.List.Operation),
			Detail:   map[string]any{"operation": r.List.Operation, "role": "list"},
		})
	} else {
		errs = append(errs, validateListBinding(r, listOp, d, s)...)
	}

	if r.Get.Operation != "" {
		getOp, ok := cat.Op(r.Get.Operation)
		if !ok {
			errs = append(errs, ValidationError{
				Type:     "unknown-operation",
				Resource: r.Key,
				Message:  fmt.Sprintf("Get operation %q is not in the spec", r.Get.Operation),
				Detail:   map[string]any{"operation": r.Get.Operation, "role": "get"},
			})
		} else {
			errs = append(errs, validateGetBinding(r, getOp)...)
		}
	}
	return errs
}

// validateListBinding checks the picker side: every fixed param exists, the
// search and paging params are real query params, and nothing the operation
// requires is left for the interpreter to invent at call time.
func validateListBinding(r Resource, op Operation, d dialectSpec, s strategy) []ValidationError {
	errs := unknownParams(r.Key, op, r.List.Params, "list")

	if r.SearchParam != "" {
		if !queryParamOK(op, d, r.SearchParam) {
			errs = append(errs, ValidationError{
				Type:     "unknown-param",
				Resource: r.Key,
				Message:  fmt.Sprintf("Search param %q is not a query parameter of %q", r.SearchParam, op.ID),
				Detail:   map[string]any{"param": r.SearchParam, "operation": op.ID},
			})
		}
	}

	if r.SelectParam != "" && !queryParamOK(op, d, r.SelectParam) {
		errs = append(errs, ValidationError{
			Type:     "unknown-param",
			Resource: r.Key,
			Message:  fmt.Sprintf("Select param %q is not a query parameter of %q", r.SelectParam, op.ID),
			Detail:   map[string]any{"param": r.SelectParam, "operation": op.ID},
		})
	}

	paging, pagingErrs := pagingParams(r, op, d, s)
	errs = append(errs, pagingErrs...)

	for _, p := range op.Params {
		if !p.Required {
			continue
		}
		if _, fixed := r.List.Params[p.Name]; fixed {
			continue
		}
		if p.Name == r.SearchParam || paging[p.Name] {
			continue
		}
		errs = append(errs, ValidationError{
			Type:     "unsatisfied-required-param",
			Resource: r.Key,
			Message:  fmt.Sprintf("List operation %q requires %q but nothing supplies it", op.ID, p.Name),
			Detail:   map[string]any{"param": p.Name, "in": p.In, "operation": op.ID, "role": "list"},
		})
	}
	return errs
}

// validateGetBinding checks the resolve side. Exactly one path param must stay
// unbound: that is the slot the stored id goes into. Everything else the
// operation requires has to be fixed at design time.
func validateGetBinding(r Resource, op Operation) []ValidationError {
	errs := unknownParams(r.Key, op, r.Get.Params, "get")

	var open []string
	for _, p := range op.PathParams() {
		if _, fixed := r.Get.Params[p.Name]; !fixed {
			open = append(open, p.Name)
		}
	}

	for _, p := range op.Params {
		if !p.Required || p.In == InPath {
			continue
		}
		if _, fixed := r.Get.Params[p.Name]; !fixed {
			errs = append(errs, ValidationError{
				Type:     "unsatisfied-required-param",
				Resource: r.Key,
				Message:  fmt.Sprintf("Get operation %q requires %q but nothing supplies it", op.ID, p.Name),
				Detail:   map[string]any{"param": p.Name, "in": p.In, "operation": op.ID, "role": "get"},
			})
		}
	}

	switch len(open) {
	case 1:
	case 0:
		errs = append(errs, ValidationError{
			Type:     "no-id-param",
			Resource: r.Key,
			Message:  fmt.Sprintf("Get operation %q has no free path parameter to carry the id", op.ID),
			Detail:   map[string]any{"operation": op.ID},
		})
	default:
		errs = append(errs, ValidationError{
			Type:     "ambiguous-id-param",
			Resource: r.Key,
			Message: fmt.Sprintf("Get operation %q leaves %s unbound; fix all but one so the id has one slot",
				op.ID, strings.Join(open, ", ")),
			Detail: map[string]any{"operation": op.ID, "open": open},
		})
	}
	return errs
}

// pagingParams returns the param names paging supplies, so the required-param
// sweep does not flag them as missing.
func pagingParams(r Resource, op Operation, d dialectSpec, s strategy) (map[string]bool, []ValidationError) {
	out := map[string]bool{}
	p := r.Pagination

	var need []string
	switch p.Style {
	case "", PageNone:
		return out, nil
	case PageOffset, PagePage:
		if p.LimitParam == "" || p.OffsetParam == "" {
			return out, []ValidationError{{
				Type:     "invalid-pagination",
				Resource: r.Key,
				Message:  fmt.Sprintf("%s paging needs both a limit param and an offset param", p.Style),
			}}
		}
		need = []string{p.LimitParam, p.OffsetParam}
	case PageCursor:
		if p.CursorParam == "" || p.CursorPath == "" {
			return out, []ValidationError{{
				Type:     "invalid-pagination",
				Resource: r.Key,
				Message:  "Cursor paging needs a cursor param and a cursor path",
			}}
		}
		if !ValidPointer(p.CursorPath) {
			return out, []ValidationError{{
				Type:     "invalid-pointer",
				Resource: r.Key,
				Message:  fmt.Sprintf("cursor_path %q is not a valid JSON pointer", p.CursorPath),
				Detail:   map[string]any{"field": "cursor_path", "pointer": p.CursorPath},
			}}
		}
		need = []string{p.CursorParam}
		if p.LimitParam != "" {
			need = append(need, p.LimitParam)
		}
	case PageLink:
		if s.linkPath == "" {
			return out, []ValidationError{{
				Type:     "invalid-pagination",
				Resource: r.Key,
				Message:  "Link paging needs a link path, and the dialect supplies no default",
			}}
		}
		if !ValidPointer(s.linkPath) {
			return out, []ValidationError{{
				Type:     "invalid-pointer",
				Resource: r.Key,
				Message:  fmt.Sprintf("link_path %q is not a valid JSON pointer", s.linkPath),
				Detail:   map[string]any{"field": "link_path", "pointer": s.linkPath},
			}}
		}
		if p.LimitParam != "" {
			need = []string{p.LimitParam}
		}
	default:
		return out, []ValidationError{{
			Type:     "invalid-pagination",
			Resource: r.Key,
			Message:  fmt.Sprintf("Unknown pagination style %q", p.Style),
			Detail:   map[string]any{"style": p.Style},
		}}
	}

	var errs []ValidationError
	for _, name := range need {
		out[name] = true
		if !queryParamOK(op, d, name) {
			errs = append(errs, ValidationError{
				Type:     "unknown-param",
				Resource: r.Key,
				Message:  fmt.Sprintf("Pagination param %q is not a query parameter of %q", name, op.ID),
				Detail:   map[string]any{"param": name, "operation": op.ID},
			})
		}
	}
	return out, errs
}

// unknownParams flags author-fixed params the operation does not declare, which
// is nearly always a rename in the spec rather than a typo.
func unknownParams(resourceKey string, op Operation, fixed map[string]string, role string) []ValidationError {
	var errs []ValidationError
	names := make([]string, 0, len(fixed))
	for name := range fixed {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if _, ok := op.Param(name); !ok {
			errs = append(errs, ValidationError{
				Type:     "unknown-param",
				Resource: resourceKey,
				Message:  fmt.Sprintf("Operation %q has no parameter %q", op.ID, name),
				Detail:   map[string]any{"param": name, "operation": op.ID, "role": role},
			})
		}
	}
	return errs
}

// validateStrategy checks the per-resource execution strategies that no
// operation can confirm on its own.
func validateStrategy(r Resource, s strategy) []ValidationError {
	var errs []ValidationError

	switch s.keyStyle {
	case KeyRaw, KeyQuoted, KeyTyped:
	default:
		errs = append(errs, ValidationError{
			Type:     "invalid-key-style",
			Resource: r.Key,
			Message:  fmt.Sprintf("Unknown key style %q; known: raw, quoted, typed", s.keyStyle),
			Detail:   map[string]any{"key_style": s.keyStyle},
		})
	}

	if r.SearchTemplate != "" {
		switch {
		case r.SearchParam == "":
			errs = append(errs, ValidationError{
				Type:     "invalid-search-template",
				Resource: r.Key,
				Message:  "A search template needs a search param to carry the expression",
			})
		case !strings.Contains(r.SearchTemplate, searchPlaceholder):
			errs = append(errs, ValidationError{
				Type:     "invalid-search-template",
				Resource: r.Key,
				Message:  fmt.Sprintf("Search template %q has no %s placeholder", r.SearchTemplate, searchPlaceholder),
			})
		}
	}
	return errs
}

// queryParamOK reports whether name may be sent as a query parameter: either
// the operation declares it, or the protocol defines it for every collection.
// Without a dialect nothing is implied, so an undeclared name is still a typo.
func queryParamOK(op Operation, d dialectSpec, name string) bool {
	if p, ok := op.Param(name); ok {
		return p.In == InQuery
	}
	return d.impliesParam(name)
}
