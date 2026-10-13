package template

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

const invoiceYAML = `name: Invoice
filename: something-else.yaml
fields:
  - key: number
    type: text
    label: Number
  - key: amount
    type: number
    label: Amount
`

type createdSpy struct{ names []string }

func (c *createdSpy) OnTemplateCreated(name string) error {
	c.names = append(c.names, name)
	return nil
}

func TestCreateFromYAML_AddsATemplateLikeTheEditorWould(t *testing.T) {
	m, _, _ := newTestManager(t)
	spy := &createdSpy{}
	m.AddCreationObserver(spy)

	if err := m.CreateFromYAML("invoice.yaml", []byte(invoiceYAML)); err != nil {
		t.Fatalf("CreateFromYAML: %v", err)
	}
	got, err := m.LoadTemplate("invoice.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Invoice" || len(got.Fields) != 2 || got.Fields[0].Key != "number" {
		t.Errorf("template = %+v", got)
	}
	if got.Filename != "invoice.yaml" {
		t.Errorf("filename = %q, want the name it was created under", got.Filename)
	}
	if !m.Exists("invoice.yaml") || len(spy.names) != 1 || spy.names[0] != "invoice.yaml" {
		t.Errorf("exists = %v, announced = %v", m.Exists("invoice.yaml"), spy.names)
	}
}

func TestCreateFromYAML_NeverReplacesAnExistingTemplate(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.CreateFromYAML("invoice.yaml", []byte(invoiceYAML)); err != nil {
		t.Fatal(err)
	}
	before, _ := m.Source("invoice.yaml")

	other := strings.Replace(invoiceYAML, "name: Invoice", "name: Hijacked", 1)
	err := m.CreateFromYAML("invoice.yaml", []byte(other))
	if !errors.Is(err, ErrExists) {
		t.Fatalf("err = %v, want ErrExists", err)
	}
	if after, _ := m.Source("invoice.yaml"); after != before {
		t.Error("the existing template was changed")
	}
}

func TestCreateFromYAML_Unhappy(t *testing.T) {
	dupKeys := "name: Bad\nfields:\n  - key: a\n    type: text\n    label: A\n  - key: a\n    type: text\n    label: A again\n"
	cases := map[string]struct {
		name, yaml string
		invalid    bool
	}{
		"folder in the name": {"../invoice.yaml", invoiceYAML, true},
		"nested name":        {"sub/invoice.yaml", invoiceYAML, true},
		"no extension":       {"invoice", invoiceYAML, true},
		"only the extension": {".yaml", invoiceYAML, true},
		"hidden file":        {".invoice.yaml", invoiceYAML, true},
		"empty name":         {"", invoiceYAML, true},
		"empty yaml":         {"invoice.yaml", "  \n", false},
		"not yaml":           {"invoice.yaml", "\t: [", false},
		"fails validation":   {"invoice.yaml", dupKeys, false},
	}
	for label, tc := range cases {
		t.Run(label, func(t *testing.T) {
			m, _, _ := newTestManager(t)
			err := m.CreateFromYAML(tc.name, []byte(tc.yaml))
			if err == nil {
				t.Fatal("created")
			}
			if tc.invalid != errors.Is(err, ErrInvalidName) {
				t.Errorf("err = %v, ErrInvalidName expected: %v", err, tc.invalid)
			}
			if list, _ := m.ListTemplates(); len(list) != 0 {
				t.Errorf("a refused create left %v behind", list)
			}
		})
	}
}

func TestCreateFromYAML_ValidationFailureIsStructured(t *testing.T) {
	m, _, _ := newTestManager(t)
	dup := "name: Bad\nfields:\n  - key: a\n    type: text\n    label: A\n  - key: a\n    type: text\n    label: B\n"
	var vf *ValidationFailedError
	if err := m.CreateFromYAML("bad.yaml", []byte(dup)); !errors.As(err, &vf) {
		t.Fatalf("err = %v, want *ValidationFailedError", err)
	}
}

func TestCreateFromYAML_ConcurrentCreatesMakeExactlyOne(t *testing.T) {
	m, _, _ := newTestManager(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	created, existed := 0, 0
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := m.CreateFromYAML("invoice.yaml", []byte(invoiceYAML))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				created++
			case errors.Is(err, ErrExists):
				existed++
			}
		}()
	}
	wg.Wait()
	if created != 1 || existed != 7 {
		t.Errorf("created = %d, refused as existing = %d; want 1 and 7", created, existed)
	}
}

func TestParseYAML_NormalizesWithoutTouchingDisk(t *testing.T) {
	got, err := ParseYAML([]byte(invoiceYAML))
	if err != nil || got.Name != "Invoice" || len(got.Fields) != 2 {
		t.Fatalf("template = %+v, err = %v", got, err)
	}
	if _, err := ParseYAML([]byte("name: Empty\n")); err != nil {
		t.Errorf("a template with no fields yet must parse: %v", err)
	}
}

func TestSourceAndExists(t *testing.T) {
	m, _, _ := newTestManager(t)
	if m.Exists("invoice.yaml") {
		t.Error("exists before it was created")
	}
	if _, err := m.Source("invoice.yaml"); err == nil {
		t.Error("source of a missing template")
	}
	if err := m.CreateFromYAML("invoice.yaml", []byte(invoiceYAML)); err != nil {
		t.Fatal(err)
	}
	src, err := m.Source("invoice.yaml")
	if err != nil || !strings.Contains(src, "name: Invoice") {
		t.Errorf("source = %q, %v", src, err)
	}
	if _, err := m.Source("../secrets.yaml"); !errors.Is(err, ErrInvalidName) {
		t.Errorf("err = %v", err)
	}
	if m.Exists("../invoice.yaml") {
		t.Error("a name outside the folder reads as existing")
	}
}

func TestReplaceFromYAML_OverwritesAndTellsUpdateObservers(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.CreateFromYAML("invoice.yaml", []byte(invoiceYAML)); err != nil {
		t.Fatal(err)
	}
	var sawOld, sawNew string
	m.AddUpdateObserver(UpdateObserverFunc(func(name string, old, updated *Template) error {
		sawOld, sawNew = old.Name, updated.Name
		return nil
	}))
	improved := strings.Replace(invoiceYAML, "name: Invoice", "name: Invoice v2", 1) +
		"  - key: due\n    type: date\n    label: Due\n"
	if err := m.ReplaceFromYAML("invoice.yaml", []byte(improved)); err != nil {
		t.Fatalf("ReplaceFromYAML: %v", err)
	}
	got, _ := m.LoadTemplate("invoice.yaml")
	if got.Name != "Invoice v2" || len(got.Fields) != 3 || got.Filename != "invoice.yaml" {
		t.Errorf("template = %+v", got)
	}
	if sawOld != "Invoice" || sawNew != "Invoice v2" {
		t.Errorf("update observer saw %q -> %q", sawOld, sawNew)
	}
}

func TestReplaceFromYAML_Unhappy(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.ReplaceFromYAML("absent.yaml", []byte(invoiceYAML)); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing template: err = %v", err)
	}
	if list, _ := m.ListTemplates(); len(list) != 0 {
		t.Error("replacing a missing template created it")
	}
	if err := m.CreateFromYAML("invoice.yaml", []byte(invoiceYAML)); err != nil {
		t.Fatal(err)
	}
	before, _ := m.Source("invoice.yaml")
	for label, tc := range map[string][2]string{
		"bad name": {"../invoice.yaml", invoiceYAML},
		"not yaml": {"invoice.yaml", "\t: ["},
		"empty":    {"invoice.yaml", " "},
		"invalid":  {"invoice.yaml", "name: Bad\nfields:\n  - key: a\n    type: text\n    label: A\n  - key: a\n    type: text\n    label: B\n"},
	} {
		if err := m.ReplaceFromYAML(tc[0], []byte(tc[1])); err == nil {
			t.Errorf("%s: replaced", label)
		}
	}
	if after, _ := m.Source("invoice.yaml"); after != before {
		t.Error("a refused replace changed the template")
	}
}

func TestDiffersFromYAML(t *testing.T) {
	m, _, _ := newTestManager(t)
	m.SetAuthorReader(fixedAuthor{})
	if err := m.CreateFromYAML("invoice.yaml", []byte(invoiceYAML)); err != nil {
		t.Fatal(err)
	}
	same := map[string]string{
		"the same text":         invoiceYAML,
		"another filename line": strings.Replace(invoiceYAML, "something-else.yaml", "whatever.yaml", 1),
		"keys in another order": "fields:\n  - label: Number\n    type: text\n    key: number\n  - type: number\n    key: amount\n    label: Amount\nname: Invoice\n",
		"another author":        invoiceYAML + "author_name: Someone\nauthor_email: s@example.com\n",
	}
	for label, src := range same {
		if differs, err := m.DiffersFromYAML("invoice.yaml", []byte(src)); err != nil || differs {
			t.Errorf("%s: differs = %v, err = %v; want the same", label, differs, err)
		}
	}
	changed := map[string]string{
		"a renamed template": strings.Replace(invoiceYAML, "name: Invoice", "name: Invoice v2", 1),
		"an added field":     invoiceYAML + "  - key: due\n    type: date\n    label: Due\n",
		"a changed label":    strings.Replace(invoiceYAML, "label: Amount", "label: Total", 1),
	}
	for label, src := range changed {
		if differs, err := m.DiffersFromYAML("invoice.yaml", []byte(src)); err != nil || !differs {
			t.Errorf("%s: differs = %v, err = %v; want a difference", label, differs, err)
		}
	}
	if _, err := m.DiffersFromYAML("absent.yaml", []byte(invoiceYAML)); err == nil {
		t.Error("comparing against a missing template gave an answer")
	}
	if _, err := m.DiffersFromYAML("invoice.yaml", []byte("\t: [")); err == nil {
		t.Error("comparing against text that is not YAML gave an answer")
	}
	// Comparing must not alter what LoadTemplate hands out next.
	if got, _ := m.LoadTemplate("invoice.yaml"); got.AuthorName == "" {
		t.Error("the comparison blanked the author on the cached template")
	}
}

type fixedAuthor struct{}

func (fixedAuthor) Author() (string, string) { return "Peter", "p@example.com" }
