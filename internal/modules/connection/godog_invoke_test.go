package connection

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/cucumber/godog"
)

// remote is a scriptable stand-in for the service a connection points at.
type remote struct {
	status      int
	contentType string
	body        string
	// bodyFn answers per request, for scripting a paged sequence.
	bodyFn func(*http.Request) string
	// base is the server's own URL, so a fixture can build absolute next links.
	base string

	path   string
	query  url.Values
	header http.Header
}

func (rm *remote) handler(w http.ResponseWriter, r *http.Request) {
	rm.path = r.URL.Path
	rm.query = r.URL.Query()
	rm.header = r.Header.Clone()
	body := rm.body
	if rm.bodyFn != nil {
		body = rm.bodyFn(r)
	}
	w.Header().Set("Content-Type", rm.contentType)
	w.WriteHeader(rm.status)
	_, _ = w.Write([]byte(body))
}

func initInvokeSteps(ctx *godog.ScenarioContext, w *connWorld) {
	ctx.Step(`^a remote that answers with:$`, func(doc *godog.DocString) error {
		if w.remote == nil {
			w.remote = &remote{status: 200, contentType: "application/json"}
		}
		w.remote.body = doc.Content
		return nil
	})

	ctx.Step(`^the remote answers with status (\d+)$`, func(status int) error {
		w.remote.status = status
		return nil
	})

	ctx.Step(`^the remote answers with an HTML page$`, func() error {
		w.remote.contentType = "text/html"
		w.remote.body = "<html><body>please log in</body></html>"
		return nil
	})

	ctx.Step(`^a connection bound to it declaring the fields "([^"]*)"$`, func(list string) error {
		srv := httptest.NewServer(http.HandlerFunc(w.remote.handler))
		w.srv = srv

		w.fs = newMemFS()
		w.mgr = NewManager(w.fs, nil)
		if _, err := w.mgr.ImportSpec("shop", []byte(specShop)); err != nil {
			return err
		}

		pointers := map[string]string{"email": "/email", "city": "/address/city", "vip": "/vip"}
		var fields []FieldMap
		for _, key := range splitList(list) {
			ptr, ok := pointers[key]
			if !ok {
				return fmt.Errorf("scenario asks for an unmapped field %q", key)
			}
			fields = append(fields, FieldMap{Key: key, Pointer: ptr})
		}

		conn := &Connection{
			ID: "shop", Name: "Shop", BaseURL: srv.URL, SpecFile: "shop.json",
			Auth: Auth{Kind: AuthNone},
			Resources: []Resource{{
				Key:         "customers",
				List:        OpRef{Operation: "listCustomers", Params: map[string]string{"tenant": "acme"}},
				Get:         OpRef{Operation: "getCustomer", Params: map[string]string{"tenant": "acme"}},
				ItemsPath:   "/data",
				IDPath:      "/id",
				LabelPath:   "/name",
				SearchParam: "q",
				Pagination:  Pagination{Style: PageOffset, LimitParam: "limit", OffsetParam: "offset"},
				Fields:      fields,
			}},
		}
		if err := w.mgr.Save(conn); err != nil {
			return err
		}
		w.inv = NewInvoker(w.mgr, staticSecret{value: "s3cret"})
		return nil
	})

	ctx.Step(`^I try the operation "([^"]*)" with:$`, func(op string, table *godog.Table) error {
		params := map[string]string{}
		for _, row := range table.Rows {
			if len(row.Cells) != 2 {
				return fmt.Errorf("a parameter row needs a name and a value")
			}
			params[strings.TrimSpace(row.Cells[0].Value)] = strings.TrimSpace(row.Cells[1].Value)
		}
		w.try, w.callErr = w.inv.Try(context.Background(),
			TryRequest{Connection: "shop", Operation: op, Params: params})
		return nil
	})

	ctx.Step(`^the try returned status (\d+)$`, func(status int) error {
		if w.callErr != nil {
			return fmt.Errorf("the call failed: %w", w.callErr)
		}
		if w.try.Status != status {
			return fmt.Errorf("status = %d, want %d", w.try.Status, status)
		}
		return nil
	})

	ctx.Step(`^the try is marked failed$`, func() error {
		if w.try == nil || !w.try.Failed {
			return errors.New("want the result marked as a failure")
		}
		return nil
	})

	ctx.Step(`^the try body contains "([^"]*)"$`, func(want string) error {
		if w.callErr != nil {
			return fmt.Errorf("the call failed: %w", w.callErr)
		}
		if !strings.Contains(w.try.Body, want) {
			return fmt.Errorf("body does not contain %q:\n%s", want, w.try.Body)
		}
		return nil
	})

	ctx.Step(`^the try body is not JSON$`, func() error {
		if w.try == nil || w.try.JSON {
			return errors.New("want the body reported as not JSON")
		}
		return nil
	})

	ctx.Step(`^the try was refused with "([^"]*)"$`, func(want string) error {
		var ie *InvokeError
		if !errors.As(w.callErr, &ie) {
			return fmt.Errorf("want a typed refusal, got %v", w.callErr)
		}
		if string(ie.Code) != want {
			return fmt.Errorf("code = %q, want %q", ie.Code, want)
		}
		return nil
	})

	ctx.Step(`^I list "([^"]*)"$`, func(res string) error {
		w.page, w.callErr = w.inv.List(context.Background(), ListRequest{Connection: "shop", Resource: res})
		return nil
	})

	ctx.Step(`^I list "([^"]*)" selecting "([^"]*)"$`, func(res, sel string) error {
		w.page, w.callErr = w.inv.List(context.Background(), ListRequest{
			Connection: "shop", Resource: res, Select: splitList(sel),
		})
		return nil
	})

	ctx.Step(`^I list "([^"]*)" searching "([^"]*)" with limit (\d+) and offset (\d+)$`,
		func(res, search string, limit, offset int) error {
			w.page, w.callErr = w.inv.List(context.Background(), ListRequest{
				Connection: "shop", Resource: res, Search: search, Limit: limit, Offset: offset,
			})
			return nil
		})

	ctx.Step(`^I fetch "([^"]*)" from "([^"]*)" selecting "([^"]*)"$`, func(id, res, sel string) error {
		w.item, w.callErr = w.inv.Fetch(context.Background(), FetchRequest{
			Connection: "shop", Resource: res, ID: id, Select: splitList(sel),
		})
		return nil
	})

	ctx.Step(`^I get (\d+) items?$`, func(n int) error {
		if w.callErr != nil {
			return w.callErr
		}
		if got := len(w.page.Items); got != n {
			return fmt.Errorf("item count = %d, want %d: %+v", got, n, w.page.Items)
		}
		return nil
	})

	ctx.Step(`^item (\d+) has id "([^"]*)" and label "([^"]*)"$`, func(idx int, id, label string) error {
		it, err := w.itemAt(idx)
		if err != nil {
			return err
		}
		if it.ID != id || it.Label != label {
			return fmt.Errorf("item %d = %q/%q, want %q/%q", idx, it.ID, it.Label, id, label)
		}
		return nil
	})

	ctx.Step(`^item (\d+) has (\d+) fields?$`, func(idx, n int) error {
		it, err := w.itemAt(idx)
		if err != nil {
			return err
		}
		if got := len(it.Fields); got != n {
			return fmt.Errorf("item %d field count = %d, want %d: %+v", idx, got, n, it.Fields)
		}
		return nil
	})

	ctx.Step(`^item (\d+) field "([^"]*)" is "([^"]*)"$`, func(idx int, key, want string) error {
		it, err := w.itemAt(idx)
		if err != nil {
			return err
		}
		if got := it.Fields[key]; got != want {
			return fmt.Errorf("item %d field %q = %q, want %q", idx, key, got, want)
		}
		return nil
	})

	ctx.Step(`^item (\d+) has no field "([^"]*)"$`, func(idx int, key string) error {
		it, err := w.itemAt(idx)
		if err != nil {
			return err
		}
		if v, present := it.Fields[key]; present {
			return fmt.Errorf("item %d unexpectedly carries %q = %q", idx, key, v)
		}
		return nil
	})

	ctx.Step(`^the fetched item has id "([^"]*)" and label "([^"]*)"$`, func(id, label string) error {
		if w.callErr != nil {
			return w.callErr
		}
		if w.item.ID != id || w.item.Label != label {
			return fmt.Errorf("fetched = %q/%q, want %q/%q", w.item.ID, w.item.Label, id, label)
		}
		return nil
	})

	ctx.Step(`^the fetched item field "([^"]*)" is "([^"]*)"$`, func(key, want string) error {
		if w.callErr != nil {
			return w.callErr
		}
		if got := w.item.Fields[key]; got != want {
			return fmt.Errorf("fetched field %q = %q, want %q", key, got, want)
		}
		return nil
	})

	ctx.Step(`^the call failed with "([^"]*)"$`, func(want string) error {
		var ie *InvokeError
		if !errors.As(w.callErr, &ie) {
			return fmt.Errorf("want an InvokeError, got %v", w.callErr)
		}
		if string(ie.Code) != want {
			return fmt.Errorf("code = %q, want %q", ie.Code, want)
		}
		return nil
	})

	ctx.Step(`^the request path was "([^"]*)"$`, func(want string) error {
		if w.remote.path != want {
			return fmt.Errorf("path = %q, want %q", w.remote.path, want)
		}
		return nil
	})

	ctx.Step(`^the request query "([^"]*)" was "([^"]*)"$`, func(key, want string) error {
		if got := w.remote.query.Get(key); got != want {
			return fmt.Errorf("query %q = %q, want %q", key, got, want)
		}
		return nil
	})

	ctx.Step(`^the request header "([^"]*)" was "([^"]*)"$`, func(key, want string) error {
		if got := w.remote.header.Get(key); got != want {
			return fmt.Errorf("header %q = %q, want %q", key, got, want)
		}
		return nil
	})
}

func (w *connWorld) itemAt(idx int) (Item, error) {
	if w.callErr != nil {
		return Item{}, w.callErr
	}
	if idx < 1 || idx > len(w.page.Items) {
		return Item{}, fmt.Errorf("item %d out of range (have %d)", idx, len(w.page.Items))
	}
	return w.page.Items[idx-1], nil
}

func splitList(s string) []string {
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}
