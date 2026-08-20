package connection

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// The test console runs one operation straight from the document, with no
// resource in between. That is the only way to answer "what does this endpoint
// actually return" for an operation nothing binds yet, which is exactly the
// question a resource binding is built to answer.
//
// It differs from the binding path in three deliberate ways: a failing status
// is a result rather than an error, a non-JSON body comes back verbatim instead
// of being refused, and an oversized body is truncated instead of fatal. All
// three exist because the console displays what happened; it does not consume
// it.

// safeMethods are the methods the console may send. Reading is idempotent, so
// a mistyped parameter costs nothing. A write is not undoable and a button that
// can fire one at a production service is a hazard, not a feature.
var safeMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodHead:    true,
	http.MethodOptions: true,
}

// maxTryBody caps what the console keeps for display, independent of the
// invoker's own limit. Past this, more text tells the author nothing.
const maxTryBody = 256 * 1024

// TryRequest runs one catalog operation. Params are keyed by parameter name as
// the spec declares them, whatever their location.
type TryRequest struct {
	Connection string            `json:"connection"`
	Operation  string            `json:"operation"`
	Params     map[string]string `json:"params,omitempty"`
}

// TryResult is one console round trip, including an unhappy one.
type TryResult struct {
	Method      string `json:"method"`
	URL         string `json:"url"`
	Status      int    `json:"status"`
	StatusText  string `json:"status_text,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Body        string `json:"body,omitempty"`

	// JSON says the body parsed, in which case Body is indented for reading.
	JSON bool `json:"json"`

	// Failed marks a 4xx or 5xx. The call still happened and the body is still
	// worth showing, so this is a flag rather than an error.
	Failed bool `json:"failed"`

	// Truncated says the body was cut for display.
	Truncated bool `json:"truncated,omitempty"`

	// DurationMS is how long the round trip took.
	DurationMS int64 `json:"duration_ms"`
}

// TryParam is one input the console has to render for an operation.
type TryParam struct {
	Name     string `json:"name"`
	In       string `json:"in"`
	Type     string `json:"type,omitempty"`
	Required bool   `json:"required"`

	// Value is what a resource already fixes for this parameter, when one
	// binds the operation. Pre-filling it means trying a bound operation
	// reproduces the call the field would make, rather than a bare one.
	Value string `json:"value,omitempty"`
}

// TryForm is everything needed to render the console for one operation.
type TryForm struct {
	Operation Operation  `json:"operation"`
	Params    []TryParam `json:"params,omitempty"`

	// Runnable is false when the console refuses the operation outright, and
	// Reason says why. Disabling the button beats a failure after the click.
	Runnable bool   `json:"runnable"`
	Reason   string `json:"reason,omitempty"`
}

// BuildTryForm describes what running an operation would need. Values a
// resource already fixes are filled in, so trying a bound operation reproduces
// the call the binding makes.
func BuildTryForm(cat *Catalog, c *Connection, opID string) (TryForm, error) {
	if cat == nil {
		return TryForm{}, invokeErr(CodeConnectionNotFound, "no parsed document", nil)
	}
	op, ok := cat.Op(opID)
	if !ok {
		return TryForm{}, invokeErr(CodeOperationNotFound,
			fmt.Sprintf("operation %q is not in the document", opID), nil)
	}

	fixed := fixedParamsFor(c, opID)
	params := make([]TryParam, 0, len(op.Params))
	for _, p := range op.Params {
		params = append(params, TryParam{
			Name:     p.Name,
			In:       p.In,
			Type:     p.Type,
			Required: p.Required,
			Value:    fixed[p.Name],
		})
	}

	form := TryForm{Operation: op, Params: params, Runnable: true}
	if !safeMethods[op.Method] {
		form.Runnable = false
		form.Reason = "method_not_allowed"
	}
	return form, nil
}

// fixedParamsFor collects what any resource already binds for an operation, so
// the console starts from the real call rather than an empty one. A parameter
// two resources fix differently takes the first in resource order, which is
// stable and visible in the form.
func fixedParamsFor(c *Connection, opID string) map[string]string {
	out := map[string]string{}
	if c == nil {
		return out
	}
	for _, r := range c.Resources {
		for _, ref := range []OpRef{r.List, r.Get} {
			if ref.Operation != opID {
				continue
			}
			for name, value := range ref.Params {
				if _, taken := out[name]; !taken {
					out[name] = value
				}
			}
		}
	}
	return out
}

// Try runs one operation and reports what came back, whatever came back.
func (iv *Invoker) Try(ctx context.Context, req TryRequest) (*TryResult, error) {
	if iv == nil || iv.mgr == nil {
		return nil, invokeErr(CodeConnectionNotFound, "no connection store", nil)
	}
	conn, cat, err := iv.mgr.Get(req.Connection)
	if err != nil {
		return nil, invokeErr(CodeConnectionNotFound,
			fmt.Sprintf("connection %q is not available", req.Connection), err)
	}
	op, ok := cat.Op(req.Operation)
	if !ok {
		return nil, invokeErr(CodeOperationNotFound,
			fmt.Sprintf("operation %q is not in the document", req.Operation), nil)
	}
	if !safeMethods[op.Method] {
		return nil, invokeErr(CodeMethodNotAllowed,
			fmt.Sprintf("%s is not a safe method; the console only reads", op.Method), nil)
	}

	params, err := overlayParams(op, fixedParamsFor(conn, op.ID), req.Params, "")
	if err != nil {
		return nil, err
	}
	if err := checkRequiredParams(op, params); err != nil {
		return nil, err
	}

	// A bare binding: no resource, so only the connection and the dialect
	// defaults shape the URL.
	b := &binding{conn: conn, catalog: cat, strat: strategyFor(conn, Resource{})}
	target, err := iv.buildURL(b, op, params, "", url.Values{})
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, op.Method, target, nil)
	if err != nil {
		return nil, invokeErr(CodeBindingInvalid, "cannot build the request", err)
	}
	httpReq.Header.Set("Accept", "application/json")
	for k, v := range conn.Headers {
		httpReq.Header.Set(k, v)
	}
	for name, value := range params {
		p, ok := op.Param(name)
		if !ok {
			continue
		}
		switch p.In {
		case InHeader:
			httpReq.Header.Set(name, value)
		case InCookie:
			httpReq.AddCookie(&http.Cookie{Name: name, Value: value})
		}
	}
	if err := iv.applyAuth(httpReq, conn); err != nil {
		return nil, err
	}

	started := time.Now()
	resp, err := iv.client.Do(httpReq)
	if err != nil {
		return nil, transportErr(ctx, err)
	}
	defer resp.Body.Close()

	raw, truncated, err := readForDisplay(resp.Body, iv.maxBytes)
	if err != nil {
		return nil, err
	}

	res := &TryResult{
		Method:      op.Method,
		URL:         redactQuery(target, conn),
		Status:      resp.StatusCode,
		StatusText:  resp.Status,
		ContentType: resp.Header.Get("Content-Type"),
		Failed:      resp.StatusCode >= 400,
		Truncated:   truncated,
		DurationMS:  time.Since(started).Milliseconds(),
	}
	res.Body, res.JSON = renderBody(raw, truncated)
	return res, nil
}

// checkRequiredParams refuses a call the document says cannot succeed, so a
// missing path parameter costs no round trip and reads as the configuration
// problem it is rather than a remote 404.
func checkRequiredParams(op Operation, params map[string]string) error {
	var missing []string
	for _, p := range op.Params {
		if !p.Required {
			continue
		}
		if strings.TrimSpace(params[p.Name]) == "" {
			missing = append(missing, p.Name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return invokeErr(CodeMissingParam,
		fmt.Sprintf("operation %q needs %s", op.ID, strings.Join(missing, ", ")), nil)
}

// readForDisplay reads at most limit bytes and says whether more were waiting.
// Unlike the binding path, an oversized body is cut rather than refused: the
// console shows a response, it does not project one.
func readForDisplay(r io.Reader, limit int64) ([]byte, bool, error) {
	if limit <= 0 || limit > maxTryBody {
		limit = maxTryBody
	}
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, false, invokeErr(CodeBadResponse, "cannot read the response body", err)
	}
	if int64(len(body)) > limit {
		return body[:limit], true, nil
	}
	return body, false, nil
}

// renderBody indents JSON for reading and passes anything else through. A
// truncated body is never parsed: half a document is not valid JSON, and
// showing the raw prefix is more useful than reporting a parse error.
func renderBody(raw []byte, truncated bool) (string, bool) {
	if truncated || len(bytes.TrimSpace(raw)) == 0 {
		return string(raw), false
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return string(raw), false
	}
	return buf.String(), true
}

// redactQuery hides a query-placed API key, so a screenshot of the console
// cannot leak the credential the vault went to the trouble of protecting.
func redactQuery(target string, conn *Connection) string {
	if conn == nil || conn.Auth.Kind != AuthAPIKey || conn.Auth.In != InQuery || conn.Auth.Name == "" {
		return target
	}
	u, err := url.Parse(target)
	if err != nil {
		return target
	}
	q := u.Query()
	if !q.Has(conn.Auth.Name) {
		return target
	}
	q.Set(conn.Auth.Name, "***")
	u.RawQuery = q.Encode()
	return u.String()
}
