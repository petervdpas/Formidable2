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
