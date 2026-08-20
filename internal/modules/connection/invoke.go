package connection

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTimeout          = 30 * time.Second
	defaultMaxResponseBytes = 8 << 20
)

// SecretResolver hands the invoker the secret for a connection. This is the
// seam the key vault plugs into: today one string per connection, later a
// client id and secret pair without the callers here changing.
type SecretResolver interface {
	Secret(connectionID string) (string, error)
}

// KeychainAccount is the keychain entry a connection's secret lives under.
// Connections are app scoped rather than profile scoped, so the first segment
// is the literal "app" where the git and gigot backends put a profile name.
func KeychainAccount(connectionID string) string {
	return "app:connection:" + connectionID
}

// Item is one remote record reduced to what a form can hold: an id to store, a
// label to show, and whichever declared fields the caller asked for.
type Item struct {
	ID     string            `json:"id"`
	Label  string            `json:"label"`
	Fields map[string]string `json:"fields,omitempty"`
}

// Page is one slice of a list operation. NextCursor is set only for
// cursor-paged resources and is empty on the last page.
type Page struct {
	Items      []Item `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// ListRequest asks a resource for candidates. Select names the declared fields
// to project; empty means every field the resource declares.
type ListRequest struct {
	Connection string   `json:"connection"`
	Resource   string   `json:"resource"`
	Search     string   `json:"search,omitempty"`
	Limit      int      `json:"limit,omitempty"`
	Offset     int      `json:"offset,omitempty"`
	Cursor     string   `json:"cursor,omitempty"`
	Select     []string `json:"select,omitempty"`
	// Params are the caller's own operation parameters, laid over the resource
	// binding's. The binding carries what is always true of the client (a tenant
	// header, an api version); Params carry what this call is asking for.
	Params map[string]string `json:"params,omitempty"`
}

// FetchRequest resolves one stored id back to a record.
type FetchRequest struct {
	Connection string            `json:"connection"`
	Resource   string            `json:"resource"`
	ID         string            `json:"id"`
	Select     []string          `json:"select,omitempty"`
	Params     map[string]string `json:"params,omitempty"`
}

// Invoker executes bindings against a remote service. It never generates code:
// every call is assembled from the catalog and the resource binding at runtime.
type Invoker struct {
	mgr      *Manager
	secrets  SecretResolver
	client   *http.Client
	log      *slog.Logger
	maxBytes int64
}

type InvokerOption func(*Invoker)

func WithTimeout(d time.Duration) InvokerOption {
	return func(iv *Invoker) { iv.client.Timeout = d }
}

func WithMaxResponseBytes(n int64) InvokerOption {
	return func(iv *Invoker) { iv.maxBytes = n }
}

func WithLogger(l *slog.Logger) InvokerOption {
	return func(iv *Invoker) { iv.log = l }
}

func NewInvoker(m *Manager, secrets SecretResolver, opts ...InvokerOption) *Invoker {
	iv := &Invoker{
		mgr:      m,
		secrets:  secrets,
		maxBytes: defaultMaxResponseBytes,
		client: &http.Client{
			Timeout:       defaultTimeout,
			CheckRedirect: refuseCrossHostRedirect,
		},
	}
	for _, opt := range opts {
		opt(iv)
	}
	return iv
}

// refuseCrossHostRedirect stops a remote from bouncing a credentialed request
// to a host the author never approved. Go strips Authorization across hosts by
// itself, but not a custom API-key header, so the redirect is refused outright.
func refuseCrossHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}
	if req.URL.Host != via[0].URL.Host {
		return fmt.Errorf("connection: refusing redirect to another host (%s)", req.URL.Host)
	}
	if len(via) >= 5 {
		return errors.New("connection: too many redirects")
	}
	return nil
}

// List runs a resource's list binding and projects each item.
func (iv *Invoker) List(ctx context.Context, req ListRequest) (*Page, error) {
	b, err := iv.bind(req.Connection, req.Resource, req.Select)
	if err != nil {
		return nil, err
	}
	op, ok := b.catalog.Op(b.resource.List.Operation)
	if !ok {
		return nil, invokeErr(CodeBindingInvalid,
			fmt.Sprintf("list operation %q is no longer in the spec", b.resource.List.Operation), nil)
	}

	query := url.Values{}
	if b.strat.searchParam != "" && req.Search != "" {
		value := req.Search
		if b.strat.searchTmpl != "" {
			value = applySearchTemplate(b.strat.searchTmpl, req.Search)
		}
		query.Set(b.strat.searchParam, value)
	}
	applyPaging(query, b.strat.paging, req)
	if b.strat.selectParam != "" {
		if names, ok := b.selectNames(); ok {
			query.Set(b.strat.selectParam, strings.Join(names, ","))
		}
	}

	// Link paging hands back a whole URL. Following it verbatim is the point:
	// a service may encode state in it that no parameter set can reproduce.
	target := ""
	if b.strat.paging.Style == PageLink && req.Cursor != "" {
		target, err = b.followURL(req.Cursor)
		if err != nil {
			return nil, err
		}
		query = nil
	}

	params, err := overlayParams(op, b.resource.List.Params, req.Params, "")
	if err != nil {
		return nil, err
	}

	doc, err := iv.call(ctx, b, op, params, "", query, target)
	if err != nil {
		return nil, err
	}

	node, ok := ResolveNode(doc, b.resource.ItemsPath)
	if !ok {
		return nil, invokeErr(CodeShapeMismatch,
			fmt.Sprintf("items_path %q does not resolve in the response", b.resource.ItemsPath), nil)
	}
	items, err := b.readItems(node)
	if err != nil {
		return nil, err
	}

	page := &Page{Items: items}
	switch b.strat.paging.Style {
	case PageCursor:
		page.NextCursor, _ = ResolvePointer(doc, b.strat.paging.CursorPath)
	case PageLink:
		page.NextCursor, _ = ResolvePointer(doc, b.strat.linkPath)
	}
	return page, nil
}

// Fetch resolves one id through a resource's get binding.
func (iv *Invoker) Fetch(ctx context.Context, req FetchRequest) (*Item, error) {
	b, err := iv.bind(req.Connection, req.Resource, req.Select)
	if err != nil {
		return nil, err
	}
	if b.resource.Get.Operation == "" {
		return nil, invokeErr(CodeBindingInvalid,
			fmt.Sprintf("resource %q has no get binding", req.Resource), nil)
	}
	if strings.TrimSpace(req.ID) == "" {
		return nil, invokeErr(CodeBindingInvalid, "no id to resolve", nil)
	}
	op, ok := b.catalog.Op(b.resource.Get.Operation)
	if !ok {
		return nil, invokeErr(CodeBindingInvalid,
			fmt.Sprintf("get operation %q is no longer in the spec", b.resource.Get.Operation), nil)
	}

	query := url.Values{}
	if b.strat.selectParam != "" {
		if names, ok := b.selectNames(); ok {
			query.Set(b.strat.selectParam, strings.Join(names, ","))
		}
	}

	// The id owns the one path placeholder the binding leaves open, so a runtime
	// param must not be able to redirect a fetch at a different record.
	idParam := ""
	for _, p := range op.PathParams() {
		if _, bound := b.resource.Get.Params[p.Name]; !bound {
			idParam = p.Name
			break
		}
	}
	params, err := overlayParams(op, b.resource.Get.Params, req.Params, idParam)
	if err != nil {
		return nil, err
	}

	doc, err := iv.call(ctx, b, op, params, req.ID, query, "")
	if err != nil {
		return nil, err
	}
	// A keyed collection has no id inside the record: the caller asked for one
	// key and that key is the record's identity.
	if b.resource.ItemsMode == ItemsMap {
		item := b.itemOf(req.ID, doc)
		return &item, nil
	}
	item, ok := b.project(doc)
	if !ok {
		return nil, invokeErr(CodeShapeMismatch,
			fmt.Sprintf("id_path %q does not resolve in the fetched record", b.resource.IDPath), nil)
	}
	return &item, nil
}

// binding is one resolved call site: the connection, the resource, its catalog,
// and the field maps the caller selected.
type binding struct {
	conn     *Connection
	catalog  *Catalog
	resource Resource
	fields   []FieldMap
	strat    strategy
}

// bind loads and checks everything that does not need the network, so a
// misconfiguration never costs a round trip.
func (iv *Invoker) bind(connID, resKey string, sel []string) (*binding, error) {
	if iv == nil || iv.mgr == nil {
		return nil, invokeErr(CodeConnectionNotFound, "no connection store", nil)
	}
	conn, cat, err := iv.mgr.Get(connID)
	if err != nil {
		return nil, invokeErr(CodeConnectionNotFound,
			fmt.Sprintf("connection %q is not available", connID), err)
	}
	res, ok := conn.Resource(resKey)
	if !ok {
		return nil, invokeErr(CodeResourceNotFound,
			fmt.Sprintf("connection %q has no resource %q", connID, resKey), nil)
	}

	fields := res.Fields
	if len(sel) > 0 {
		fields = make([]FieldMap, 0, len(sel))
		for _, key := range sel {
			f, ok := res.Field(key)
			if !ok {
				return nil, invokeErr(CodeUnknownField,
					fmt.Sprintf("resource %q does not declare a field %q", resKey, key), nil)
			}
			fields = append(fields, f)
		}
	}
	return &binding{conn: conn, catalog: cat, resource: res, fields: fields, strat: strategyFor(conn, res)}, nil
}

// project turns one decoded remote record into an Item. A record with no
// resolvable id is not referenceable, so it is reported as unusable.
// readItems turns the resolved item container into records. An array yields one
// per entry; a keyed map yields one per key, with the key as the id, walked in
// sorted order because a JSON object has none of its own.
func (b *binding) readItems(node any) ([]Item, error) {
	if b.resource.ItemsMode == ItemsMap {
		obj, ok := node.(map[string]any)
		if !ok {
			return nil, invokeErr(CodeShapeMismatch,
				fmt.Sprintf("items_path %q resolves to %T, not a keyed object", b.resource.ItemsPath, node), nil)
		}
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		out := make([]Item, 0, len(keys))
		for _, k := range keys {
			if k == "" {
				continue
			}
			out = append(out, b.itemOf(k, obj[k]))
		}
		return out, nil
	}

	raw, ok := node.([]any)
	if !ok {
		return nil, invokeErr(CodeShapeMismatch,
			fmt.Sprintf("items_path %q resolves to %T, not a list", b.resource.ItemsPath, node), nil)
	}
	out := make([]Item, 0, len(raw))
	for _, entry := range raw {
		item, ok := b.project(entry)
		if !ok {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func (b *binding) project(node any) (Item, bool) {
	id, ok := ResolvePointer(node, b.resource.IDPath)
	if !ok || id == "" {
		return Item{}, false
	}
	return b.itemOf(id, node), true
}

// itemOf builds a record around an id already established, so the keyed and
// array paths project the label and fields identically.
func (b *binding) itemOf(id string, node any) Item {
	// An unset label pointer means there is no label, not "the whole record":
	// resolving "" would hand back the entire node, which reads as a JSON blob
	// in a picker and as the raw value in a keyed collection of scalars.
	label := id
	if b.resource.LabelPath != "" {
		if v, ok := ResolvePointer(node, b.resource.LabelPath); ok && v != "" {
			label = v
		}
	}

	item := Item{ID: id, Label: label}
	for _, f := range b.fields {
		if v, ok := ResolvePointer(node, f.Pointer); ok {
			if item.Fields == nil {
				item.Fields = make(map[string]string, len(b.fields))
			}
			item.Fields[f.Key] = v
		}
	}
	return item
}

// applyPaging writes only the params the resource actually declares, so a
// caller passing a limit at a resource without paging sends nothing extra.
func applyPaging(query url.Values, p Pagination, req ListRequest) {
	switch p.Style {
	case PageOffset, PagePage:
		if req.Limit > 0 {
			query.Set(p.LimitParam, strconv.Itoa(req.Limit))
		}
		if req.Offset > 0 {
			query.Set(p.OffsetParam, strconv.Itoa(req.Offset))
		}
	case PageCursor:
		if req.Cursor != "" {
			query.Set(p.CursorParam, req.Cursor)
		}
		if p.LimitParam != "" && req.Limit > 0 {
			query.Set(p.LimitParam, strconv.Itoa(req.Limit))
		}
	}
}

// call assembles and runs one request, returning the decoded JSON body. pathID
// fills the single free path param of a get binding; it is empty for a list.
func (iv *Invoker) call(
	ctx context.Context,
	b *binding,
	op Operation,
	fixed map[string]string,
	pathID string,
	query url.Values,
	absolute string,
) (any, error) {
	target := absolute
	if target == "" {
		built, err := iv.buildURL(b, op, fixed, pathID, query)
		if err != nil {
			return nil, err
		}
		target = built
	}

	req, err := http.NewRequestWithContext(ctx, op.Method, target, nil)
	if err != nil {
		return nil, invokeErr(CodeBindingInvalid, "cannot build the request", err)
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range b.conn.Headers {
		req.Header.Set(k, v)
	}
	for name, value := range fixed {
		p, ok := op.Param(name)
		if !ok {
			continue
		}
		switch p.In {
		case InHeader:
			req.Header.Set(name, value)
		case InCookie:
			req.AddCookie(&http.Cookie{Name: name, Value: value})
		}
	}
	if err := iv.applyAuth(req, b.conn); err != nil {
		return nil, err
	}

	resp, err := iv.client.Do(req)
	if err != nil {
		return nil, transportErr(ctx, err)
	}
	defer resp.Body.Close()

	body, err := iv.readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, &InvokeError{
			Code:    statusCode(resp.StatusCode),
			Message: fmt.Sprintf("%s %s returned %s", op.Method, op.Path, resp.Status),
			Status:  resp.StatusCode,
		}
	}
	return decodeJSON(resp, body)
}

// buildURL substitutes path params and appends the query. Values are escaped
// per component, so an id containing a slash cannot invent a path segment.
// overlayParams lays the caller's params over the binding's. A name the operation
// does not declare is an error rather than a silent drop: a field configured
// against a stale spec would otherwise quietly return unfiltered data. reserved
// names a param the caller may not set (the fetch id's path placeholder).
func overlayParams(op Operation, fixed, runtime map[string]string, reserved string) (map[string]string, error) {
	if len(runtime) == 0 {
		return fixed, nil
	}
	out := make(map[string]string, len(fixed)+len(runtime))
	for k, v := range fixed {
		out[k] = v
	}
	for k, v := range runtime {
		if k == reserved {
			continue
		}
		if _, ok := op.Param(k); !ok {
			return nil, invokeErr(CodeUnknownField,
				fmt.Sprintf("operation %q declares no parameter %q", op.ID, k), nil)
		}
		out[k] = v
	}
	return out, nil
}

func (iv *Invoker) buildURL(
	b *binding,
	op Operation,
	fixed map[string]string,
	pathID string,
	query url.Values,
) (string, error) {
	base := strings.TrimSuffix(b.conn.BaseURL, "/")
	if base == "" {
		base = FirstAbsoluteServer(b.catalog)
	}
	if base == "" {
		return "", invokeErr(CodeBindingInvalid, "connection has no base URL", nil)
	}

	opPath := op.Path
	for _, p := range op.PathParams() {
		value, ok := fixed[p.Name]
		if !ok {
			if pathID == "" {
				return "", invokeErr(CodeBindingInvalid,
					fmt.Sprintf("path parameter %q has no value", p.Name), nil)
			}
			value = pathID
		}
		opPath = strings.ReplaceAll(opPath, "{"+p.Name+"}",
			url.PathEscape(formatKey(b.strat.keyStyle, p, value)))
	}
	if strings.ContainsAny(opPath, "{}") {
		return "", invokeErr(CodeBindingInvalid,
			fmt.Sprintf("path %q still has an unfilled placeholder", op.Path), nil)
	}

	for name, value := range fixed {
		if p, ok := op.Param(name); ok && p.In == InQuery {
			query.Set(name, value)
		}
	}
	if b.conn.Auth.Kind == AuthAPIKey && b.conn.Auth.In == InQuery {
		secret, err := iv.secret(b.conn)
		if err != nil {
			return "", err
		}
		query.Set(b.conn.Auth.Name, secret)
	}

	u, err := url.Parse(base + opPath)
	if err != nil {
		return "", invokeErr(CodeBindingInvalid, fmt.Sprintf("cannot build a URL from %q", base+opPath), err)
	}
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}
	return u.String(), nil
}

// applyAuth attaches the credential. A query-placed API key is already on the
// URL by this point, so it is skipped here.
func (iv *Invoker) applyAuth(req *http.Request, conn *Connection) error {
	switch conn.Auth.Kind {
	case "", AuthNone:
		return nil
	case AuthAPIKey:
		if conn.Auth.In == InQuery {
			return nil
		}
		secret, err := iv.secret(conn)
		if err != nil {
			return err
		}
		req.Header.Set(conn.Auth.Name, secret)
	case AuthBearer:
		secret, err := iv.secret(conn)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+secret)
	case AuthBasic:
		secret, err := iv.secret(conn)
		if err != nil {
			return err
		}
		req.SetBasicAuth(conn.Auth.User, secret)
	}
	return nil
}

// secret reads the credential, reporting a missing one as configuration rather
// than as a transport failure: the user has to go and enter it.
func (iv *Invoker) secret(conn *Connection) (string, error) {
	if iv.secrets == nil {
		return "", invokeErr(CodeNotConfigured,
			fmt.Sprintf("connection %q needs a credential and none is available", conn.ID), nil)
	}
	secret, err := iv.secrets.Secret(conn.ID)
	if err != nil || secret == "" {
		return "", invokeErr(CodeNotConfigured,
			fmt.Sprintf("no credential stored for connection %q", conn.ID), err)
	}
	return secret, nil
}

// readBody reads at most maxBytes+1, so an oversized payload is refused rather
// than buffered whole.
func (iv *Invoker) readBody(resp *http.Response) ([]byte, error) {
	limit := iv.maxBytes
	if limit <= 0 {
		limit = defaultMaxResponseBytes
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, invokeErr(CodeBadResponse, "cannot read the response body", err)
	}
	if int64(len(body)) > limit {
		return nil, invokeErr(CodeTooLarge,
			fmt.Sprintf("response exceeds the %d byte limit", limit), nil)
	}
	return body, nil
}

// decodeJSON refuses a non-JSON payload outright: an HTML login page answering
// 200 is the usual cause, and parsing it would report a shape problem instead
// of the authentication problem it really is.
func decodeJSON(resp *http.Response, body []byte) (any, error) {
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		mt, _, err := mime.ParseMediaType(ct)
		if err == nil && !strings.Contains(mt, "json") {
			return nil, invokeErr(CodeBadResponse,
				fmt.Sprintf("expected JSON, got %s", mt), nil)
		}
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, invokeErr(CodeBadResponse, "response is not valid JSON", err)
	}
	return doc, nil
}

// transportErr separates the caller giving up from the remote being slow or
// absent, so the UI can say something true about which happened.
func transportErr(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); errors.Is(ctxErr, context.Canceled) {
		return invokeErr(CodeCanceled, "the request was cancelled", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return invokeErr(CodeTimeout, "the remote did not answer in time", err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return invokeErr(CodeTimeout, "the remote did not answer in time", err)
	}
	return invokeErr(CodeUnreachable, "cannot reach the remote service", err)
}

// selectNames is the property list a server-side projection should ask for. It
// must include the id and label or the service would strip the very fields the
// reference is built from. ok is false when any pointer covers the whole
// record, in which case no projection can be narrowed safely.
func (b *binding) selectNames() ([]string, bool) {
	var names []string
	seen := map[string]bool{}

	add := func(name string) bool {
		if name == "" {
			return false
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
		return true
	}

	if !add(pointerToProperty(b.resource.IDPath)) {
		return nil, false
	}
	if !add(pointerToProperty(b.resource.LabelPath)) {
		return nil, false
	}
	for _, f := range b.fields {
		if !add(remoteName(f)) {
			return nil, false
		}
	}
	return names, true
}

// followURL resolves a next-page link against the connection base and refuses
// to leave the host. A link is remote-controlled input: without this, a service
// could point the next page at anywhere and be handed the credentials.
func (b *binding) followURL(link string) (string, error) {
	base := strings.TrimSuffix(b.conn.BaseURL, "/")
	if base == "" {
		base = FirstAbsoluteServer(b.catalog)
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", invokeErr(CodeBindingInvalid, "connection has no usable base URL", err)
	}
	next, err := url.Parse(link)
	if err != nil {
		return "", invokeErr(CodeBindingInvalid, "next page link is not a URL", err)
	}
	resolved := baseURL.ResolveReference(next)
	if resolved.Host != baseURL.Host {
		return "", invokeErr(CodeBindingInvalid,
			fmt.Sprintf("next page link points at %s, not %s", resolved.Host, baseURL.Host), nil)
	}
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return "", invokeErr(CodeBindingInvalid,
			fmt.Sprintf("next page link uses the %q scheme", resolved.Scheme), nil)
	}
	return resolved.String(), nil
}
