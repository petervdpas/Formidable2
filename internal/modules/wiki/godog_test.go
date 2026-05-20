package wiki

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
	"github.com/petervdpas/formidable2/internal/modules/expression"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	tpl "github.com/petervdpas/formidable2/internal/modules/template"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initWikiScenario,
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

// facetsFromTable parses a godog data table into []tpl.Facet. Layout:
//
//	| key | icon     | LABEL1,color1 | LABEL2,color2 | ...
//
// Cells after the first two are option specs "LABEL,COLOR". Whitespace
// is trimmed; an empty cell ends the option list for that row.
func facetsFromTable(table *godog.Table) ([]tpl.Facet, error) {
	out := make([]tpl.Facet, 0, len(table.Rows))
	for i, row := range table.Rows {
		if len(row.Cells) < 3 {
			return nil, fmt.Errorf("facet row %d needs at least key|icon|option", i)
		}
		f := tpl.Facet{
			Key:  strings.TrimSpace(row.Cells[0].Value),
			Icon: strings.TrimSpace(row.Cells[1].Value),
		}
		for _, cell := range row.Cells[2:] {
			raw := strings.TrimSpace(cell.Value)
			if raw == "" {
				continue
			}
			parts := strings.SplitN(raw, ",", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("facet option %q must be `LABEL,color`", raw)
			}
			f.Options = append(f.Options, tpl.FacetOption{
				Label: strings.TrimSpace(parts[0]),
				Color: strings.TrimSpace(parts[1]),
			})
		}
		out = append(out, f)
	}
	return out, nil
}

func colorWriter() io.Writer {
	if w, ok := any(os.Stdout).(io.Writer); ok {
		return w
	}
	return io.Discard
}

// world is the per-scenario state — kept tiny on purpose. The manager
// under test, the second one used for the port-conflict scenario, the
// loose values we thread between Given/When/Then, and (Slice 2) the
// wiki handler under test plus its last-recorded HTTP response.
type world struct {
	m            *Manager
	mSecond      *Manager
	startErr     error
	stopErr      error
	rememberPort int

	// Slice 2 — read-path routes
	handler *Handler
	stub    *stubProvider
	stubEx  *stubExpressioner
	resp    *httptest.ResponseRecorder

	// Slice 3 — /storage/*
	stubSt *stubStorage

	// Slice 5 — facets surface (Templates interface + form facet state)
	stubTpl *stubTemplates

	// Slice 4 — Service surface
	svc            *Service
	configPort     int
	rememberSvcPort int
	browserURL     string
	windowURL      string
	actionErr      error
}

func (w *world) reset() {
	if w.m != nil {
		_ = w.m.Stop()
	}
	if w.mSecond != nil {
		_ = w.mSecond.Stop()
	}
	if w.svc != nil {
		_ = w.svc.StopServer()
	}
	*w = world{}
}

func initWikiScenario(ctx *godog.ScenarioContext) {
	w := &world{}

	ctx.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w.reset()
		return ctx, nil
	})

	ctx.Step(`^a wiki manager$`, func() error {
		w.m = NewManager(nil)
		return nil
	})

	ctx.Step(`^a custom handler returning "([^"]*)"$`, func(body string) error {
		w.m.SetHandler(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(rw, body)
		}))
		return nil
	})

	ctx.Step(`^I start the server on a random port$`, func() error {
		w.startErr = w.m.Start(0)
		return nil
	})

	ctx.Step(`^the server has started on a random port$`, func() error {
		if err := w.m.Start(0); err != nil {
			return fmt.Errorf("seed start: %w", err)
		}
		return nil
	})

	ctx.Step(`^I remember the bound port$`, func() error {
		s := w.m.Status()
		if s.Port == 0 {
			return fmt.Errorf("no port to remember")
		}
		w.rememberPort = s.Port
		return nil
	})

	ctx.Step(`^I stop the server$`, func() error {
		w.stopErr = w.m.Stop()
		return nil
	})

	ctx.Step(`^I start the server on the remembered port$`, func() error {
		w.startErr = w.m.Start(w.rememberPort)
		return nil
	})

	ctx.Step(`^a second manager tries to start on the remembered port$`, func() error {
		w.mSecond = NewManager(nil)
		w.startErr = w.mSecond.Start(w.rememberPort)
		return nil
	})

	// ── Thens ─────────────────────────────────────────────────────────

	ctx.Step(`^the server is not running$`, func() error {
		if w.m.Status().Running {
			return fmt.Errorf("expected not running")
		}
		return nil
	})

	ctx.Step(`^the reported port is zero$`, func() error {
		if got := w.m.Status().Port; got != 0 {
			return fmt.Errorf("port = %d, want 0", got)
		}
		return nil
	})

	ctx.Step(`^the server is running$`, func() error {
		if !w.m.Status().Running {
			return fmt.Errorf("expected running, startErr=%v", w.startErr)
		}
		return nil
	})

	ctx.Step(`^the reported port is non-zero$`, func() error {
		if w.m.Status().Port == 0 {
			return fmt.Errorf("expected non-zero port")
		}
		return nil
	})

	ctx.Step(`^the started-at timestamp is set$`, func() error {
		if w.m.Status().StartedAt.IsZero() {
			return fmt.Errorf("expected non-zero StartedAt")
		}
		return nil
	})

	ctx.Step(`^HTTP GET on "([^"]*)" returns a response$`, func(path string) error {
		port := w.m.Status().Port
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, path))
		if err != nil {
			return fmt.Errorf("get: %w", err)
		}
		_ = resp.Body.Close()
		return nil
	})

	ctx.Step(`^HTTP GET on "([^"]*)" returns body "([^"]*)"$`, func(path, want string) error {
		port := w.m.Status().Port
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, path))
		if err != nil {
			return fmt.Errorf("get: %w", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read body: %w", err)
		}
		if string(body) != want {
			return fmt.Errorf("body = %q, want %q", body, want)
		}
		return nil
	})

	ctx.Step(`^HTTP GET on "([^"]*)" fails$`, func(path string) error {
		port := w.rememberPort
		if port == 0 {
			port = 1 // random invalid; we expect the GET to fail
		}
		_, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, path))
		if err == nil {
			return fmt.Errorf("expected error, got nil")
		}
		return nil
	})

	ctx.Step(`^no error is returned$`, func() error {
		if w.startErr != nil {
			return fmt.Errorf("startErr = %v", w.startErr)
		}
		if w.stopErr != nil {
			return fmt.Errorf("stopErr = %v", w.stopErr)
		}
		return nil
	})

	ctx.Step(`^a start error is returned$`, func() error {
		if w.startErr == nil {
			return fmt.Errorf("expected start error, got nil")
		}
		return nil
	})

	ctx.Step(`^the bound port matches the remembered port$`, func() error {
		got := w.m.Status().Port
		if got != w.rememberPort {
			return fmt.Errorf("port = %d, want %d", got, w.rememberPort)
		}
		return nil
	})

	ctx.Step(`^the status is not running with port zero$`, func() error {
		s := w.m.Status()
		if s.Running {
			return fmt.Errorf("expected not running")
		}
		if s.Port != 0 {
			return fmt.Errorf("port = %d, want 0", s.Port)
		}
		return nil
	})

	// ── Slice 2: read-path routes ────────────────────────────────────

	ctx.Step(`^a wiki handler over a stub dataprovider with two templates$`, func() error {
		w.stub = newStubProvider()
		// Trim the stub's seeded forms so a downstream "the dataprovider
		// has forms for X: ..." step shapes them explicitly.
		w.stub.forms = map[string][]dataprovider.FormSummary{}
		w.stubSt = newStubStorage()
		w.stubEx = &stubExpressioner{items: map[string][]expression.Result{}}
		w.stubTpl = &stubTemplates{byName: map[string]*tpl.Template{}}
		w.handler = NewHandler(w.stub, w.stubSt, w.stubEx)
		w.handler.SetTemplates(w.stubTpl)
		return nil
	})

	// ── Facets — Templates + storage facet state ─────────────────────

	ctx.Step(`^the template "([^"]*)" declares facets:$`,
		func(filename string, table *godog.Table) error {
			if w.stubTpl == nil {
				return fmt.Errorf("templates stub not initialised")
			}
			facets, err := facetsFromTable(table)
			if err != nil {
				return err
			}
			w.stubTpl.byName[filename] = &tpl.Template{
				Filename: filename,
				Facets:   facets,
			}
			return nil
		})

	ctx.Step(`^the form "([^"]*)" has facets:$`,
		func(formKey string, table *godog.Table) error {
			if w.stubSt == nil {
				return fmt.Errorf("storage stub not initialised")
			}
			parts := strings.SplitN(formKey, "/", 2)
			if len(parts) != 2 {
				return fmt.Errorf("form key %q must be `<template>/<datafile>`", formKey)
			}
			states := make(map[string]storage.FacetState, len(table.Rows))
			for _, row := range table.Rows {
				if len(row.Cells) < 3 {
					return fmt.Errorf("form facet row needs 3 cells: key|set|selected")
				}
				states[row.Cells[0].Value] = storage.FacetState{
					Set:      strings.EqualFold(strings.TrimSpace(row.Cells[1].Value), "true"),
					Selected: strings.TrimSpace(row.Cells[2].Value),
				}
			}
			w.stubSt.forms[formKey] = &storage.Form{
				Meta: storage.FormMeta{
					Template: parts[0],
					Facets:   states,
				},
			}
			return nil
		})

	ctx.Step(`^the expression engine yields for "([^"]*)" \(filename, text\):$`,
		func(template string, table *godog.Table) error {
			if w.stubEx == nil {
				return fmt.Errorf("expression stub not initialised")
			}
			items := make([]expression.Result, 0, len(table.Rows))
			for _, row := range table.Rows {
				if len(row.Cells) < 2 {
					continue
				}
				items = append(items, expression.Result{
					Filename: row.Cells[0].Value,
					Text:     row.Cells[1].Value,
				})
			}
			w.stubEx.items[template] = items
			return nil
		})

	// ── Slice 3: /storage/* static handler ────────────────────────────

	ctx.Step(`^a wiki handler with a stub storage holding "([^"]*)" → "([^"]*)" of "([^"]*)"$`,
		func(stem, name, body string) error {
			w.stub = newStubProvider()
			w.stubSt = &stubStorage{
				images: map[string][]byte{
					stem + ".yaml/" + name: []byte(body),
				},
			}
			w.handler = NewHandler(w.stub, w.stubSt, &stubExpressioner{})
			return nil
		})

	ctx.Step(`^the response body is "([^"]*)"$`, func(want string) error {
		got := w.resp.Body.String()
		if got != want {
			return fmt.Errorf("body = %q, want %q", got, want)
		}
		return nil
	})

	ctx.Step(`^the dataprovider has forms for "([^"]*)": "([^"]*)", "([^"]*)"$`,
		func(template, a, b string) error {
			if w.stub == nil {
				return fmt.Errorf("stub not initialised")
			}
			w.stub.forms[template] = []dataprovider.FormSummary{
				{Template: template, Filename: a, Title: a},
				{Template: template, Filename: b, Title: b},
			}
			return nil
		})

	// ── EnabledTemplate filter ─────────────────────────────────────

	ctx.Step(`^the wiki filter enables only "([^"]*)"$`, func(csv string) error {
		if w.handler == nil {
			return fmt.Errorf("wiki handler not initialised")
		}
		allowed := make([]string, 0)
		for _, p := range strings.Split(csv, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				allowed = append(allowed, p)
			}
		}
		w.handler.SetEnabledFilter(&stubFilter{allowed: allowed})
		return nil
	})

	ctx.Step(`^the wiki filter enables nothing$`, func() error {
		if w.handler == nil {
			return fmt.Errorf("wiki handler not initialised")
		}
		w.handler.SetEnabledFilter(&stubFilter{allowed: []string{}})
		return nil
	})

	ctx.Step(`^the dataprovider has an image "([^"]*)" under "([^"]*)" with body "([^"]*)"$`,
		func(name, tpl, body string) error {
			if w.stubSt == nil {
				return fmt.Errorf("storage stub not initialised")
			}
			w.stubSt.images[tpl+"/"+name] = []byte(body)
			return nil
		})

	ctx.Step(`^I GET "([^"]*)"$`, func(path string) error {
		w.resp = httptest.NewRecorder()
		w.handler.ServeHTTP(w.resp, httptest.NewRequest(http.MethodGet, path, nil))
		return nil
	})

	ctx.Step(`^I POST "([^"]*)"$`, func(path string) error {
		w.resp = httptest.NewRecorder()
		w.handler.ServeHTTP(w.resp, httptest.NewRequest(http.MethodPost, path, nil))
		return nil
	})

	ctx.Step(`^the response status is (\d+)$`, func(want int) error {
		if w.resp == nil {
			return fmt.Errorf("no response recorded")
		}
		if w.resp.Code != want {
			return fmt.Errorf("status = %d, want %d", w.resp.Code, want)
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

	ctx.Step(`^the html links to "([^"]*)"$`, func(href string) error {
		body := w.resp.Body.String()
		needle := `href="` + href + `"`
		if !strings.Contains(body, needle) {
			return fmt.Errorf("body missing %q", needle)
		}
		return nil
	})

	ctx.Step(`^the html shows the template name "([^"]*)"$`, func(name string) error {
		if !strings.Contains(w.resp.Body.String(), name) {
			return fmt.Errorf("body missing template name %q", name)
		}
		return nil
	})

	ctx.Step(`^the html body contains "([^"]*)"$`, func(needle string) error {
		if !strings.Contains(w.resp.Body.String(), needle) {
			return fmt.Errorf("body missing %q", needle)
		}
		return nil
	})

	ctx.Step(`^the html body does not contain "([^"]*)"$`, func(needle string) error {
		if strings.Contains(w.resp.Body.String(), needle) {
			return fmt.Errorf("body unexpectedly contains %q", needle)
		}
		return nil
	})

	ctx.Step(`^the html body has element id "([^"]*)"$`, func(id string) error {
		needle := `id="` + id + `"`
		if !strings.Contains(w.resp.Body.String(), needle) {
			return fmt.Errorf("body missing %q", needle)
		}
		return nil
	})

	ctx.Step(`^the html body does not contain "([^"]*)"$`, func(needle string) error {
		if strings.Contains(w.resp.Body.String(), needle) {
			return fmt.Errorf("body should not contain %q", needle)
		}
		return nil
	})

	ctx.Step(`^the dataprovider renders "([^"]*)" with body containing a wiki link to "([^"]*)" "([^"]*)"$`,
		func(currentDatafile, linkStem, linkDatafile string) error {
			if w.stub == nil {
				return fmt.Errorf("stub not initialised")
			}
			// The wiki context's render.Manager already rewrote the
			// href before the handler sees it — that's what these
			// scenarios verify the handler doesn't unintentionally
			// undo.
			href := "/template/" + linkStem + "/form/" + linkDatafile
			w.stub.render = func(_, datafile string) (*dataprovider.RenderedPage, error) {
				return &dataprovider.RenderedPage{
					Filename: datafile,
					Title:    "scenario",
					HTML:     `<p>see <a href="` + href + `">other</a></p>`,
				}, nil
			}
			return nil
		})

	// ── Slice 4: Service surface ─────────────────────────────────────

	ctx.Step(`^a wiki service over a stub dataprovider and a configured port$`, func() error {
		w.stub = newStubProvider()
		w.stubSt = newStubStorage()
		w.handler = NewHandler(w.stub, w.stubSt, &stubExpressioner{})
		w.m = NewManager(nil)
		w.m.SetHandler(w.handler)
		w.svc = NewService(w.m, func() int { return w.configPort },
			func(url string) error { w.browserURL = url; return nil },
			nil) // window opener installed only when scenario asks
		return nil
	})

	ctx.Step(`^the configured port is (\d+)$`, func(p int) error {
		w.configPort = p
		return nil
	})

	ctx.Step(`^the configured port changes to (\d+)$`, func(p int) error {
		w.configPort = p
		return nil
	})

	ctx.Step(`^the service has started the server$`, func() error {
		return w.svc.StartServer()
	})

	ctx.Step(`^I StartServer through the service$`, func() error {
		w.actionErr = w.svc.StartServer()
		return nil
	})

	ctx.Step(`^I StopServer through the service$`, func() error {
		w.actionErr = w.svc.StopServer()
		return nil
	})

	ctx.Step(`^I OpenInBrowser through the service$`, func() error {
		w.actionErr = w.svc.OpenInBrowser()
		return nil
	})

	ctx.Step(`^I OpenInternalWiki through the service$`, func() error {
		w.actionErr = w.svc.OpenInternalWiki()
		return nil
	})

	ctx.Step(`^a window opener is installed$`, func() error {
		InstallWindowOpener(w.svc, func(url string) error {
			w.windowURL = url
			return nil
		})
		return nil
	})

	ctx.Step(`^I remember the service port$`, func() error {
		w.rememberSvcPort = w.svc.GetServerStatus().Port
		return nil
	})

	ctx.Step(`^the service reports running$`, func() error {
		if !w.svc.GetServerStatus().Running {
			return fmt.Errorf("service not running")
		}
		return nil
	})

	ctx.Step(`^the service reports not running$`, func() error {
		if w.svc.GetServerStatus().Running {
			return fmt.Errorf("service unexpectedly running")
		}
		return nil
	})

	ctx.Step(`^the service-reported port is non-zero$`, func() error {
		if got := w.svc.GetServerStatus().Port; got == 0 {
			return fmt.Errorf("service port = 0")
		}
		return nil
	})

	ctx.Step(`^the service-reported port is zero$`, func() error {
		if got := w.svc.GetServerStatus().Port; got != 0 {
			return fmt.Errorf("service port = %d, want 0", got)
		}
		return nil
	})

	ctx.Step(`^the new service-reported port differs from the remembered one$`, func() error {
		got := w.svc.GetServerStatus().Port
		if got == w.rememberSvcPort {
			return fmt.Errorf("port did not change: still %d", got)
		}
		return nil
	})

	ctx.Step(`^the service action returned an error containing "([^"]*)"$`, func(needle string) error {
		if w.actionErr == nil {
			return fmt.Errorf("expected error, got nil")
		}
		if !strings.Contains(w.actionErr.Error(), needle) {
			return fmt.Errorf("error %q missing %q", w.actionErr.Error(), needle)
		}
		return nil
	})

	ctx.Step(`^the registered browser opener was invoked with the loopback URL$`, func() error {
		want := fmt.Sprintf("http://127.0.0.1:%d/", w.svc.GetServerStatus().Port)
		if w.browserURL != want {
			return fmt.Errorf("browser url = %q, want %q", w.browserURL, want)
		}
		return nil
	})

	ctx.Step(`^the registered window opener was invoked with the loopback URL$`, func() error {
		want := fmt.Sprintf("http://127.0.0.1:%d/", w.svc.GetServerStatus().Port)
		if w.windowURL != want {
			return fmt.Errorf("window url = %q, want %q", w.windowURL, want)
		}
		return nil
	})

	ctx.Step(`^I HTTP GET the service root URL$`, func() error {
		port := w.svc.GetServerStatus().Port
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		w.resp = httptest.NewRecorder()
		w.resp.Code = resp.StatusCode
		_, _ = io.Copy(w.resp.Body, resp.Body)
		return nil
	})

	ctx.Step(`^the live response status is (\d+)$`, func(want int) error {
		if w.resp.Code != want {
			return fmt.Errorf("status = %d, want %d", w.resp.Code, want)
		}
		return nil
	})
}
