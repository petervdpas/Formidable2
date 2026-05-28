package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initAPIScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			Output:   colorWriter(),
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fail()
	}
}

func colorWriter() io.Writer {
	if w, ok := any(os.Stdout).(io.Writer); ok {
		return w
	}
	return io.Discard
}

// world holds per-scenario state. Tiny by design - godog steps share
// it through closure capture, just like the wiki package's tests.
type world struct {
	stub    *stubProvider
	stubSt  *stubStorage
	stubWr  *stubWriter
	stubTpl *stubTemplates
	handler http.Handler
	resp    *httptest.ResponseRecorder

	// Decoded JSON for "the JSON …" assertions. Decoded lazily on the
	// first json-related step so plain-status scenarios stay cheap.
	jsonOnce bool
	jsonAny  any

	// A2: ETag round-trip. capturedETag holds the ETag from a prior
	// "I GET ... and capture the ETag" step so a follow-up scenario
	// can hand it back as If-None-Match.
	capturedETag string
}

func (w *world) reset() {
	*w = world{}
}

// ensureJSON decodes w.resp.Body into w.jsonAny on demand. Returns
// the typed root for callers that need a specific shape.
func (w *world) ensureJSON() (any, error) {
	if w.resp == nil {
		return nil, fmt.Errorf("no response captured")
	}
	if !w.jsonOnce {
		var v any
		if err := json.Unmarshal(w.resp.Body.Bytes(), &v); err != nil {
			return nil, fmt.Errorf("decode json: %w (body=%q)", err, w.resp.Body.String())
		}
		w.jsonAny = v
		w.jsonOnce = true
	}
	return w.jsonAny, nil
}

func initAPIScenario(ctx *godog.ScenarioContext) {
	w := &world{}

	ctx.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w.reset()
		return ctx, nil
	})

	// ── Givens (handler + corpus) ─────────────────────────────────────

	ctx.Step(`^an API handler over a stub dataprovider with these templates:$`,
		func(table *godog.Table) error {
			w.stub = &stubProvider{forms: map[string][]dataprovider.FormSummary{}}
			w.stubSt = newStubStorage()
			// Wire the stub provider to the same storage so facet
			// filtering can resolve meta.facets via LoadForm - mirrors
			// the real dp's m.sto.LoadForm path.
			w.stub.storage = w.stubSt
			w.stubWr = newStubWriter()
			// Plumb writer → storage so a POST followed by a GET sees
			// the freshly-written form (mirrors the production
			// indexer-hook flow without requiring real SQLite).
			w.stubWr.st = w.stubSt
			w.stubTpl = newStubTemplates()
			// Skip the header row.
			for _, row := range table.Rows[1:] {
				cells := row.Cells
				filename := cells[0].Value
				name := cells[1].Value
				enable := strings.EqualFold(cells[2].Value, "true")
				guid := cells[3].Value
				stem := strings.TrimSuffix(filename, ".yaml")
				w.stub.templates = append(w.stub.templates, dataprovider.TemplateSummary{
					Stem:             stem,
					Filename:         filename,
					Name:             name,
					EnableCollection: enable,
					GuidField:        guid,
				})
				// Mirror the row into the templates store so the design
				// endpoint can find both enabled and disabled templates.
				// Scenarios that need richer field/option data overlay
				// later via "the templates store has design ..." steps.
				w.stubTpl.by[filename] = &template.Template{
					Name:             name,
					Filename:         filename,
					EnableCollection: enable,
				}
			}
			w.handler = NewHandler(w.stub, w.stubSt, w.stubWr, w.stubTpl, nil, nil)
			return nil
		})

	ctx.Step(`^the dataprovider has forms for "([^"]*)":$`,
		func(template string, table *godog.Table) error {
			if w.stub == nil {
				return fmt.Errorf("stub not initialised")
			}
			rows := []dataprovider.FormSummary{}
			for _, row := range table.Rows[1:] {
				cells := row.Cells
				rows = append(rows, dataprovider.FormSummary{
					Template: template,
					Filename: cells[0].Value,
					ID:       cells[1].Value,
					Title:    cells[2].Value,
				})
			}
			w.stub.forms[template] = rows
			return nil
		})

	ctx.Step(`^the storage holds form "([^"]*)":"([^"]*)" with:$`,
		func(template, datafile string, body *godog.DocString) error {
			if w.stubSt == nil {
				return fmt.Errorf("stubSt not initialised")
			}
			var f storage.Form
			if err := json.Unmarshal([]byte(body.Content), &f); err != nil {
				return fmt.Errorf("decode form: %w", err)
			}
			w.stubSt.forms[template+"/"+datafile] = &f
			return nil
		})

	// ── A4: design endpoint givens ───────────────────────────────────

	ctx.Step(`^the templates store has design "([^"]*)":$`,
		func(filename string, table *godog.Table) error {
			if w.stubTpl == nil {
				return fmt.Errorf("stubTpl not initialised")
			}
			t := &template.Template{Filename: filename}
			for _, row := range table.Rows {
				key := strings.TrimSpace(row.Cells[0].Value)
				val := row.Cells[1].Value
				switch key {
				case "name":
					t.Name = val
				case "item_field":
					t.ItemField = val
				case "markdown_template":
					// Gherkin tables don't preserve "\n"; decode it as a
					// real newline so feature files stay readable.
					t.MarkdownTemplate = strings.ReplaceAll(val, `\n`, "\n")
				case "sidebar_expression":
					t.SidebarExpression = val
				case "enable_collection":
					t.EnableCollection = strings.EqualFold(val, "true")
				}
			}
			w.stubTpl.by[filename] = t
			return nil
		})

	ctx.Step(`^the templates store design "([^"]*)" has fields:$`,
		func(filename string, table *godog.Table) error {
			t, ok := w.stubTpl.by[filename]
			if !ok {
				// Allow scenarios that only specify fields (no preceding
				// metadata table) by lazily creating the entry.
				t = &template.Template{Filename: filename, EnableCollection: true}
				w.stubTpl.by[filename] = t
			}
			t.Fields = nil
			for _, row := range table.Rows[1:] {
				cells := row.Cells
				f := template.Field{
					Key:   cells[0].Value,
					Type:  cells[1].Value,
					Label: cells[2].Value,
				}
				if raw := strings.TrimSpace(cells[3].Value); raw != "" {
					// Parse `key:label,key:label,...` into option maps.
					// Tests use bare scalars (`a,b`) too - fall back to
					// "value=label=token" when there's no colon.
					for pair := range strings.SplitSeq(raw, ",") {
						pair = strings.TrimSpace(pair)
						if pair == "" {
							continue
						}
						value, label, hasColon := strings.Cut(pair, ":")
						if !hasColon {
							label = value
						}
						f.Options = append(f.Options, map[string]any{
							"value": value,
							"label": label,
						})
					}
				}
				t.Fields = append(t.Fields, f)
			}
			return nil
		})

	ctx.Step(`^the templates store design "([^"]*)" has facet "([^"]*)" with icon "([^"]*)" and options:$`,
		func(filename, key, icon string, table *godog.Table) error {
			t, ok := w.stubTpl.by[filename]
			if !ok {
				t = &template.Template{Filename: filename, EnableCollection: true}
				w.stubTpl.by[filename] = t
			}
			opts := []template.FacetOption{}
			for _, row := range table.Rows[1:] {
				opts = append(opts, template.FacetOption{
					Label: row.Cells[0].Value,
					Color: row.Cells[1].Value,
				})
			}
			t.Facets = append(t.Facets, template.Facet{
				Key:     key,
				Icon:    icon,
				Options: opts,
			})
			return nil
		})

	ctx.Step(`^the storage form "([^"]*)":"([^"]*)" has facet "([^"]*)" set (true|false) selected "([^"]*)"$`,
		func(tpl, datafile, key, setStr, selected string) error {
			fkey := tpl + "/" + datafile
			f, ok := w.stubSt.forms[fkey]
			if !ok {
				f = &storage.Form{}
				w.stubSt.forms[fkey] = f
			}
			if f.Meta.Facets == nil {
				f.Meta.Facets = map[string]storage.FacetState{}
			}
			f.Meta.Facets[key] = storage.FacetState{
				Set:      setStr == "true",
				Selected: selected,
			}
			return nil
		})

	ctx.Step(`^the dataprovider has tagged forms for "([^"]*)":$`,
		func(template string, table *godog.Table) error {
			if w.stub == nil {
				return fmt.Errorf("stub not initialised")
			}
			rows := []dataprovider.FormSummary{}
			for _, row := range table.Rows[1:] {
				cells := row.Cells
				var tags []string
				if raw := strings.TrimSpace(cells[3].Value); raw != "" {
					for t := range strings.SplitSeq(raw, ",") {
						if t = strings.TrimSpace(t); t != "" {
							tags = append(tags, t)
						}
					}
				}
				rows = append(rows, dataprovider.FormSummary{
					Template: template,
					Filename: cells[0].Value,
					ID:       cells[1].Value,
					Title:    cells[2].Value,
					Tags:     tags,
				})
			}
			w.stub.forms[template] = rows
			// Bump the rev so test scenarios that capture an ETag end
			// up with a non-zero, meaningful validator.
			w.stub.rev++
			return nil
		})

	// ── Whens ─────────────────────────────────────────────────────────

	ctx.Step(`^I GET "([^"]*)"$`, func(path string) error {
		w.resp = httptest.NewRecorder()
		w.jsonOnce = false
		w.handler.ServeHTTP(w.resp, httptest.NewRequest(http.MethodGet, path, nil))
		return nil
	})

	ctx.Step(`^I GET "([^"]*)" and capture the ETag$`, func(path string) error {
		rec := httptest.NewRecorder()
		w.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		etag := rec.Header().Get("ETag")
		if etag == "" {
			return fmt.Errorf("no ETag on prior response (status %d)", rec.Code)
		}
		w.capturedETag = etag
		return nil
	})

	ctx.Step(`^I GET "([^"]*)" with header "([^"]*)" matching the captured ETag$`,
		func(path, header string) error {
			if w.capturedETag == "" {
				return fmt.Errorf("no ETag captured yet")
			}
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set(header, w.capturedETag)
			w.resp = httptest.NewRecorder()
			w.jsonOnce = false
			w.handler.ServeHTTP(w.resp, req)
			return nil
		})

	ctx.Step(`^I GET "([^"]*)" with header "([^"]*)" "([^"]*)"$`,
		func(path, header, value string) error {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set(header, value)
			w.resp = httptest.NewRecorder()
			w.jsonOnce = false
			w.handler.ServeHTTP(w.resp, req)
			return nil
		})

	ctx.Step(`^I HEAD "([^"]*)"$`, func(path string) error {
		w.resp = httptest.NewRecorder()
		w.jsonOnce = false
		w.handler.ServeHTTP(w.resp, httptest.NewRequest(http.MethodHead, path, nil))
		return nil
	})

	ctx.Step(`^I POST "([^"]*)" with body:$`, func(path string, body *godog.DocString) error {
		w.resp = httptest.NewRecorder()
		w.jsonOnce = false
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body.Content))
		req.Header.Set("Content-Type", "application/json")
		w.handler.ServeHTTP(w.resp, req)
		return nil
	})

	ctx.Step(`^I PUT "([^"]*)" with body:$`, func(path string, body *godog.DocString) error {
		w.resp = httptest.NewRecorder()
		w.jsonOnce = false
		req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body.Content))
		req.Header.Set("Content-Type", "application/json")
		w.handler.ServeHTTP(w.resp, req)
		return nil
	})

	ctx.Step(`^I PATCH "([^"]*)" with body:$`, func(path string, body *godog.DocString) error {
		w.resp = httptest.NewRecorder()
		w.jsonOnce = false
		req := httptest.NewRequest(http.MethodPatch, path, strings.NewReader(body.Content))
		req.Header.Set("Content-Type", "application/json")
		w.handler.ServeHTTP(w.resp, req)
		return nil
	})

	ctx.Step(`^I PATCH "([^"]*)" with header "([^"]*)" matching the captured ETag and body:$`,
		func(path, header string, body *godog.DocString) error {
			if w.capturedETag == "" {
				return fmt.Errorf("no ETag captured yet")
			}
			req := httptest.NewRequest(http.MethodPatch, path, strings.NewReader(body.Content))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(header, w.capturedETag)
			w.resp = httptest.NewRecorder()
			w.jsonOnce = false
			w.handler.ServeHTTP(w.resp, req)
			return nil
		})

	ctx.Step(`^I PATCH "([^"]*)" with header "([^"]*)" "([^"]*)" and body:$`,
		func(path, header, value string, body *godog.DocString) error {
			req := httptest.NewRequest(http.MethodPatch, path, strings.NewReader(body.Content))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(header, value)
			w.resp = httptest.NewRecorder()
			w.jsonOnce = false
			w.handler.ServeHTTP(w.resp, req)
			return nil
		})

	ctx.Step(`^the writer recorded (\d+) save[s]?$`, func(want int) error {
		got := len(w.stubWr.saves)
		if got != want {
			return fmt.Errorf("writer saves = %d, want %d", got, want)
		}
		return nil
	})

	ctx.Step(`^I DELETE "([^"]*)"$`, func(path string) error {
		w.resp = httptest.NewRecorder()
		w.jsonOnce = false
		w.handler.ServeHTTP(w.resp, httptest.NewRequest(http.MethodDelete, path, nil))
		return nil
	})

	ctx.Step(`^the writer recorded (\d+) delete[s]?$`, func(want int) error {
		got := len(w.stubWr.deletes)
		if got != want {
			return fmt.Errorf("writer deletes = %d, want %d", got, want)
		}
		return nil
	})

	ctx.Step(`^the writer fails the next delete with "([^"]*)"$`, func(msg string) error {
		w.stubWr.delErr = errors.New(msg)
		return nil
	})

	ctx.Step(`^the JSON has a non-empty "([^"]*)" field$`, func(field string) error {
		obj, err := jsonAsObject(w)
		if err != nil {
			return err
		}
		v, ok := obj[field]
		if !ok {
			return fmt.Errorf("missing field %q", field)
		}
		s := fmt.Sprint(v)
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("field %q is empty", field)
		}
		return nil
	})

	// ── Thens (status + headers) ──────────────────────────────────────

	ctx.Step(`^the response status is (\d+)$`, func(want int) error {
		if w.resp == nil {
			return fmt.Errorf("no response captured")
		}
		if w.resp.Code != want {
			return fmt.Errorf("status = %d, want %d (body=%q)", w.resp.Code, want, w.resp.Body.String())
		}
		return nil
	})

	ctx.Step(`^the response content-type is "([^"]*)"$`, func(want string) error {
		got := w.resp.Header().Get("Content-Type")
		if got != want {
			return fmt.Errorf("content-type = %q, want %q", got, want)
		}
		return nil
	})

	ctx.Step(`^the response has header "([^"]*)"$`, func(name string) error {
		if w.resp.Header().Get(name) == "" {
			return fmt.Errorf("missing header %q", name)
		}
		return nil
	})

	ctx.Step(`^the response body is empty$`, func() error {
		if w.resp.Body.Len() != 0 {
			return fmt.Errorf("body should be empty, got %q", w.resp.Body.String())
		}
		return nil
	})

	ctx.Step(`^the body contains "([^"]*)"$`, func(needle string) error {
		body := w.resp.Body.String()
		if !strings.Contains(body, needle) {
			return fmt.Errorf("body missing %q (len=%d)", needle, len(body))
		}
		return nil
	})

	ctx.Step(`^the response body equals "([^"]*)"$`, func(want string) error {
		got := w.resp.Body.String()
		if got != want {
			return fmt.Errorf("body = %q, want %q", got, want)
		}
		return nil
	})

	ctx.Step(`^the response body starts with "([^"]*)"$`, func(prefix string) error {
		got := w.resp.Body.String()
		if !strings.HasPrefix(got, prefix) {
			return fmt.Errorf("body does not start with %q (got %q)", prefix, got)
		}
		return nil
	})

	// ── Image-route givens ───────────────────────────────────────────

	ctx.Step(`^the storage has image "([^"]*)":"([^"]*)" with bytes "([^"]*)"$`,
		func(template, filename, body string) error {
			if w.stubSt == nil {
				return fmt.Errorf("stubSt not initialised")
			}
			w.stubSt.putImage(template, filename, []byte(body))
			return nil
		})

	ctx.Step(`^the JSON has path "([^"]*)"$`, func(path string) error {
		obj, err := jsonAsObject(w)
		if err != nil {
			return err
		}
		paths, ok := obj["paths"].(map[string]any)
		if !ok {
			return fmt.Errorf("response has no `paths` object")
		}
		if _, found := paths[path]; !found {
			return fmt.Errorf("path %q not in spec (have %v)", path, mapKeys(paths))
		}
		return nil
	})

	ctx.Step(`^the spec path "([^"]*)" has method "([^"]*)" with summary "([^"]*)"$`,
		func(path, method, want string) error {
			obj, err := jsonAsObject(w)
			if err != nil {
				return err
			}
			paths, ok := obj["paths"].(map[string]any)
			if !ok {
				return fmt.Errorf("response has no `paths` object")
			}
			entry, ok := paths[path].(map[string]any)
			if !ok {
				return fmt.Errorf("path %q not in spec", path)
			}
			op, ok := entry[method].(map[string]any)
			if !ok {
				return fmt.Errorf("path %q has no %q operation", path, method)
			}
			got, _ := op["summary"].(string)
			if got != want {
				return fmt.Errorf("summary = %q, want %q", got, want)
			}
			return nil
		})

	ctx.Step(`^the spec path "([^"]*)" has method "([^"]*)"$`,
		func(path, method string) error {
			obj, err := jsonAsObject(w)
			if err != nil {
				return err
			}
			paths, ok := obj["paths"].(map[string]any)
			if !ok {
				return fmt.Errorf("response has no `paths` object")
			}
			entry, ok := paths[path].(map[string]any)
			if !ok {
				return fmt.Errorf("path %q not in spec", path)
			}
			if _, found := entry[method]; !found {
				return fmt.Errorf("path %q has no %q operation (have %v)", path, method, mapKeys(entry))
			}
			return nil
		})

	ctx.Step(`^the JSON does not have field "([^"]*)"$`, func(field string) error {
		obj, err := jsonAsObject(w)
		if err != nil {
			return err
		}
		if _, found := obj[field]; found {
			return fmt.Errorf("field %q present, want absent", field)
		}
		return nil
	})

	ctx.Step(`^the JSON does NOT have schema "([^"]*)"$`, func(name string) error {
		obj, err := jsonAsObject(w)
		if err != nil {
			return err
		}
		schemas, ok := getNested(obj, "components.schemas")
		if !ok {
			// No components/schemas at all → satisfies "does not have"
			return nil
		}
		s, ok := schemas.(map[string]any)
		if !ok {
			return nil
		}
		if _, found := s[name]; found {
			return fmt.Errorf("schema %q should not be in spec", name)
		}
		return nil
	})

	ctx.Step(`^the response header "([^"]*)" contains "([^"]*)"$`, func(name, needle string) error {
		got := w.resp.Header().Get(name)
		if !strings.Contains(got, needle) {
			return fmt.Errorf("header %q = %q, missing %q", name, got, needle)
		}
		return nil
	})

	ctx.Step(`^the body starts with the UTF-8 BOM$`, func() error {
		body := w.resp.Body.Bytes()
		if len(body) < 3 || body[0] != 0xEF || body[1] != 0xBB || body[2] != 0xBF {
			return fmt.Errorf("body does not start with UTF-8 BOM (first bytes: %x)", body[:min(3, len(body))])
		}
		return nil
	})

	// ── NDJSON / CSV line indexing ────────────────────────────────────

	ctx.Step(`^the body has (\d+) NDJSON lines$`, func(want int) error {
		got := countNDJSONLines(w.resp.Body.String())
		if got != want {
			return fmt.Errorf("ndjson lines = %d, want %d (body=%q)", got, want, w.resp.Body.String())
		}
		return nil
	})

	ctx.Step(`^NDJSON line (\d+) has "([^"]*)" == "([^"]*)"$`, func(idx int, field, want string) error {
		obj, err := ndjsonLine(w, idx)
		if err != nil {
			return err
		}
		if got, ok := obj[field]; !ok || fmt.Sprint(got) != want {
			return fmt.Errorf("line %d %s = %q, want %q", idx, field, fmt.Sprint(got), want)
		}
		return nil
	})

	ctx.Step(`^NDJSON line (\d+) nested "([^"]*)" == "([^"]*)"$`, func(idx int, path, want string) error {
		obj, err := ndjsonLine(w, idx)
		if err != nil {
			return err
		}
		got, ok := getNested(obj, path)
		if !ok {
			return fmt.Errorf("line %d path %q missing", idx, path)
		}
		if fmt.Sprint(got) != want {
			return fmt.Errorf("line %d %s = %q, want %q", idx, path, fmt.Sprint(got), want)
		}
		return nil
	})

	ctx.Step(`^CSV line (\d+) is "([^"]*)"$`, func(idx int, want string) error {
		line, err := csvLine(w, idx)
		if err != nil {
			return err
		}
		if line != want {
			return fmt.Errorf("csv line %d = %q, want %q", idx, line, want)
		}
		return nil
	})

	ctx.Step(`^CSV line (\d+) starts with "([^"]*)"$`, func(idx int, prefix string) error {
		line, err := csvLine(w, idx)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(line, prefix) {
			return fmt.Errorf("csv line %d %q has no prefix %q", idx, line, prefix)
		}
		return nil
	})

	ctx.Step(`^CSV line (\d+) contains "([^"]*)"$`, func(idx int, needle string) error {
		line, err := csvLine(w, idx)
		if err != nil {
			return err
		}
		if !strings.Contains(line, needle) {
			return fmt.Errorf("csv line %d %q missing %q", idx, line, needle)
		}
		return nil
	})

	// ── Thens (JSON shape) ────────────────────────────────────────────

	ctx.Step(`^the JSON array contains a row with id "([^"]*)" and href "([^"]*)"$`,
		func(id, href string) error {
			rows, err := jsonAsArray(w)
			if err != nil {
				return err
			}
			for _, item := range rows {
				obj, _ := item.(map[string]any)
				if obj["id"] == id && obj["href"] == href {
					return nil
				}
			}
			return fmt.Errorf("no row matching id=%q href=%q in %v", id, href, rows)
		})

	ctx.Step(`^the JSON array contains a row with id "([^"]*)"$`, func(id string) error {
		rows, err := jsonAsArray(w)
		if err != nil {
			return err
		}
		for _, item := range rows {
			obj, _ := item.(map[string]any)
			if obj["id"] == id {
				return nil
			}
		}
		return fmt.Errorf("no row with id=%q in %v", id, rows)
	})

	ctx.Step(`^the JSON array does NOT contain a row with id "([^"]*)"$`, func(id string) error {
		rows, err := jsonAsArray(w)
		if err != nil {
			return err
		}
		for _, item := range rows {
			obj, _ := item.(map[string]any)
			if obj["id"] == id {
				return fmt.Errorf("found id=%q (should be absent)", id)
			}
		}
		return nil
	})

	ctx.Step(`^the JSON row with id "([^"]*)" has name "([^"]*)"$`, func(id, want string) error {
		rows, err := jsonAsArray(w)
		if err != nil {
			return err
		}
		for _, item := range rows {
			obj, _ := item.(map[string]any)
			if obj["id"] == id {
				if obj["name"] != want {
					return fmt.Errorf("name = %q, want %q", obj["name"], want)
				}
				return nil
			}
		}
		return fmt.Errorf("no row with id=%q", id)
	})

	ctx.Step(`^the JSON has "([^"]*)" == "([^"]*)"$`, func(field, want string) error {
		obj, err := jsonAsObject(w)
		if err != nil {
			return err
		}
		got, ok := obj[field]
		if !ok {
			return fmt.Errorf("missing field %q", field)
		}
		if fmt.Sprint(got) != want {
			return fmt.Errorf("%s = %q, want %q", field, fmt.Sprint(got), want)
		}
		return nil
	})

	ctx.Step(`^the JSON "([^"]*)" has length (\d+)$`, func(field string, want int) error {
		obj, err := jsonAsObject(w)
		if err != nil {
			return err
		}
		got, ok := obj[field]
		if !ok {
			return fmt.Errorf("missing field %q", field)
		}
		arr, ok := got.([]any)
		if !ok {
			return fmt.Errorf("%s is not an array (got %T)", field, got)
		}
		if len(arr) != want {
			return fmt.Errorf("%s length = %d, want %d", field, len(arr), want)
		}
		return nil
	})

	ctx.Step(`^the JSON nested "([^"]*)" == "([^"]*)"$`, func(path, want string) error {
		obj, err := jsonAsObject(w)
		if err != nil {
			return err
		}
		// getIndexed supports both dotted paths and `arr[N]` indices,
		// so it's a strict superset of getNested. Use it everywhere.
		got, ok := getIndexed(obj, path)
		if !ok {
			return fmt.Errorf("path %q not found", path)
		}
		if fmt.Sprint(got) != want {
			return fmt.Errorf("%s = %q, want %q", path, fmt.Sprint(got), want)
		}
		return nil
	})

	ctx.Step(`^the JSON nested "([^"]*)" == (\d+)$`, func(path string, want int) error {
		obj, err := jsonAsObject(w)
		if err != nil {
			return err
		}
		got, ok := getIndexed(obj, path)
		if !ok {
			return fmt.Errorf("path %q not found", path)
		}
		// JSON numbers decode as float64.
		f, ok := got.(float64)
		if !ok {
			return fmt.Errorf("%s = %v (%T), want number", path, got, got)
		}
		if int(f) != want {
			return fmt.Errorf("%s = %v, want %d", path, f, want)
		}
		return nil
	})

	// the JSON nested fields[2] "type" == "dropdown"
	// the JSON nested fields[2].options[0] "value" == "bread"
	// `path` is a dotted/index expression rooted at the response body.
	ctx.Step(`^the JSON nested ([a-zA-Z0-9_.\[\]]+) "([^"]*)" == "([^"]*)"$`,
		func(path, leaf, want string) error {
			obj, err := jsonAsObject(w)
			if err != nil {
				return err
			}
			node, ok := getIndexed(obj, path)
			if !ok {
				return fmt.Errorf("path %q not found", path)
			}
			leafObj, ok := node.(map[string]any)
			if !ok {
				return fmt.Errorf("path %q is %T, expected object", path, node)
			}
			got, ok := leafObj[leaf]
			if !ok {
				return fmt.Errorf("leaf %q missing under %q", leaf, path)
			}
			if fmt.Sprint(got) != want {
				return fmt.Errorf("%s.%s = %q, want %q", path, leaf, fmt.Sprint(got), want)
			}
			return nil
		})

	ctx.Step(`^the JSON has "([^"]*)" == (true|false)$`, func(field, want string) error {
		obj, err := jsonAsObject(w)
		if err != nil {
			return err
		}
		got, ok := obj[field]
		if !ok {
			return fmt.Errorf("missing field %q", field)
		}
		b, ok := got.(bool)
		if !ok {
			return fmt.Errorf("%s = %v (%T), want bool", field, got, got)
		}
		if fmt.Sprint(b) != want {
			return fmt.Errorf("%s = %v, want %s", field, b, want)
		}
		return nil
	})

	ctx.Step(`^the JSON has "([^"]*)" == (\d+)$`, func(field string, want int) error {
		obj, err := jsonAsObject(w)
		if err != nil {
			return err
		}
		got, ok := obj[field]
		if !ok {
			return fmt.Errorf("missing field %q", field)
		}
		// JSON numbers decode as float64.
		f, ok := got.(float64)
		if !ok {
			return fmt.Errorf("%s = %v (%T), want number", field, got, got)
		}
		if int(f) != want {
			return fmt.Errorf("%s = %v, want %d", field, f, want)
		}
		return nil
	})
}

func jsonAsArray(w *world) ([]any, error) {
	v, err := w.ensureJSON()
	if err != nil {
		return nil, err
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("response is not a JSON array (got %T)", v)
	}
	return arr, nil
}

// mapKeys returns the sorted keys of a map[string]any. Used in
// step error messages so failures point at "what's actually there".
func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// countNDJSONLines counts non-empty lines in a body. NDJSON drops a
// trailing newline so we trim before splitting.
func countNDJSONLines(body string) int {
	body = strings.TrimRight(body, "\n")
	if body == "" {
		return 0
	}
	return strings.Count(body, "\n") + 1
}

// ndjsonLine decodes line N from an NDJSON body. Indexed from 0.
func ndjsonLine(w *world, idx int) (map[string]any, error) {
	lines := strings.Split(strings.TrimRight(w.resp.Body.String(), "\n"), "\n")
	if idx < 0 || idx >= len(lines) {
		return nil, fmt.Errorf("ndjson line %d out of range (have %d)", idx, len(lines))
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(lines[idx]), &obj); err != nil {
		return nil, fmt.Errorf("decode ndjson line %d: %w", idx, err)
	}
	return obj, nil
}

// csvLine returns line N from a CSV body, stripped of the trailing
// CR/LF and the leading UTF-8 BOM (only on line 0). Indexed from 0.
func csvLine(w *world, idx int) (string, error) {
	body := w.resp.Body.Bytes()
	// Strip BOM so line indexing is cell-first, not byte-first.
	if len(body) >= 3 && body[0] == 0xEF && body[1] == 0xBB && body[2] == 0xBF {
		body = body[3:]
	}
	// encoding/csv emits CRLF line endings; split on either to be robust.
	lines := strings.Split(strings.TrimRight(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n"), "\n")
	if idx < 0 || idx >= len(lines) {
		return "", fmt.Errorf("csv line %d out of range (have %d)", idx, len(lines))
	}
	return lines[idx], nil
}

// indexedSegment matches `name` or `name[idx]` segments of an
// indexed-path expression like `fields[2].options[0]`.
var indexedSegment = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*)(?:\[(\d+)\])?$`)

// getIndexed walks an expression like `fields[2].options[0]` through
// nested maps and arrays. Returns the raw value at the path; bails
// with ok=false on a structural mismatch (descending an array as an
// object, missing key, out-of-range index).
func getIndexed(root map[string]any, expr string) (any, bool) {
	var cur any = root
	for seg := range strings.SplitSeq(expr, ".") {
		m := indexedSegment.FindStringSubmatch(seg)
		if m == nil {
			return nil, false
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := obj[m[1]]
		if !ok {
			return nil, false
		}
		cur = v
		if m[2] != "" {
			idx, err := strconv.Atoi(m[2])
			if err != nil {
				return nil, false
			}
			arr, ok := cur.([]any)
			if !ok || idx < 0 || idx >= len(arr) {
				return nil, false
			}
			cur = arr[idx]
		}
	}
	return cur, true
}

// getNested walks a dotted path through nested maps. Bails on any
// non-map intermediate node (e.g. trying to descend into an array).
func getNested(root map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var cur any = root
	for _, p := range parts {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := obj[p]
		if !ok {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

func jsonAsObject(w *world) (map[string]any, error) {
	v, err := w.ensureJSON()
	if err != nil {
		return nil, err
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("response is not a JSON object (got %T)", v)
	}
	return obj, nil
}
