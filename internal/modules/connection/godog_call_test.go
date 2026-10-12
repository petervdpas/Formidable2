package connection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/cucumber/godog"
)

// callWorld is the state of one call.feature scenario.
type callWorld struct {
	srv    *httptest.Server
	inv    *Invoker
	status int
	got    sent
	res    *CallResult
	err    error
}

func initCallSteps(ctx *godog.ScenarioContext) {
	w := &callWorld{}

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		*w = callWorld{status: 0}
		return ctx, nil
	})
	ctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		if w.srv != nil {
			w.srv.Close()
		}
		return ctx, nil
	})

	ctx.Step(`^a remote that records each request$`, func() error {
		w.srv = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			w.got.method, w.got.path, w.got.contentType, w.got.body = r.Method, r.URL.Path, r.Header.Get("Content-Type"), string(raw)
			w.got.hits++
			status := w.status
			if status == 0 {
				status = map[string]int{http.MethodPost: 201, http.MethodDelete: 204}[r.Method]
			}
			if status == 0 {
				status = 200
			}
			if status != 204 {
				rw.Header().Set("Content-Type", "application/json")
			}
			rw.WriteHeader(status)
			if status != 204 {
				_, _ = rw.Write([]byte(`{"ok": true}`))
			}
		}))
		return nil
	})

	ctx.Step(`^the recording remote answers with status (\d+)$`, func(status int) error {
		w.status = status
		return nil
	})

	ctx.Step(`^a client for the document with write operations$`, func() error {
		mgr := NewManager(newMemFS(), nil)
		if _, err := mgr.ImportSpec("shop", []byte(writesSpec)); err != nil {
			return err
		}
		if err := mgr.Save(&Connection{ID: "shop", Name: "Shop", BaseURL: w.srv.URL, SpecFile: "shop.json"}); err != nil {
			return err
		}
		w.inv = NewInvoker(mgr, staticSecret{value: "s3cret"})
		return nil
	})

	call := func(op string, params map[string]string, body any) {
		w.res, w.err = w.inv.Call(context.Background(), CallRequest{Connection: "shop", Operation: op, Params: params, Body: body})
	}

	ctx.Step(`^the program calls "([^"]*)" with the JSON body:$`, func(op string, doc *godog.DocString) error {
		var body any
		if err := json.Unmarshal([]byte(doc.Content), &body); err != nil {
			return err
		}
		call(op, nil, body)
		return nil
	})
	ctx.Step(`^the program calls "([^"]*)" for "([^"]*)" with a PDF file as body$`, func(op, id string) error {
		call(op, map[string]string{"id": id}, realPDF)
		return nil
	})
	ctx.Step(`^the program calls "([^"]*)" with a PDF file as body$`, func(op string) error {
		call(op, nil, realPDF)
		return nil
	})
	ctx.Step(`^the program calls "([^"]*)" for "([^"]*)" without a body$`, func(op, id string) error {
		call(op, map[string]string{"id": id}, nil)
		return nil
	})
	ctx.Step(`^the program calls "([^"]*)" without a body$`, func(op string) error {
		call(op, nil, nil)
		return nil
	})
	ctx.Step(`^I try the write operation "([^"]*)" in the console$`, func(op string) error {
		_, w.err = w.inv.Try(context.Background(), TryRequest{Connection: "shop", Operation: op})
		return nil
	})

	ctx.Step(`^the call returned status (\d+)$`, func(status int) error {
		if w.err != nil {
			return fmt.Errorf("the call failed: %v", w.err)
		}
		if w.res.Status != status {
			return fmt.Errorf("status = %d, want %d", w.res.Status, status)
		}
		return nil
	})
	ctx.Step(`^the call was refused with "([^"]*)"$`, func(code string) error {
		var ie *InvokeError
		if !errors.As(w.err, &ie) {
			return fmt.Errorf("err = %v, want an invoke error", w.err)
		}
		if string(ie.Code) != code {
			return fmt.Errorf("code = %s, want %s", ie.Code, code)
		}
		return nil
	})
	ctx.Step(`^the remote received (\d+) requests?$`, func(n int) error {
		if w.got.hits != n {
			return fmt.Errorf("the remote received %d requests, want %d", w.got.hits, n)
		}
		return nil
	})
	ctx.Step(`^the remote received "([^"]*)" on "([^"]*)" as "([^"]*)"$`, func(method, path, contentType string) error {
		if w.got.method != method || w.got.path != path || w.got.contentType != contentType {
			return fmt.Errorf("received %s %s as %q", w.got.method, w.got.path, w.got.contentType)
		}
		return nil
	})
	ctx.Step(`^the remote received the body:$`, func(doc *godog.DocString) error {
		if w.got.body != strings.TrimSpace(doc.Content) {
			return fmt.Errorf("body = %q", w.got.body)
		}
		return nil
	})
	ctx.Step(`^the remote received exactly the bytes of the PDF file$`, func() error {
		if w.got.body != realPDF {
			return fmt.Errorf("%d bytes arrived, %d were sent", len(w.got.body), len(realPDF))
		}
		return nil
	})
}
