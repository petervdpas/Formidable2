package render

import (
	"sort"
	"strings"
	"testing"

	"github.com/aymerick/raymond"
)

// TestCatalog_MatchesRegisteredHelpers is the drift-guard. Every
// helper declared in builtinHelpers MUST also be registered on a
// real Handlebars template - otherwise the reference panel surfaces
// dead links to nonexistent helpers. The reverse direction (every
// registered helper is cataloged) isn't strictly checked here, but
// the symmetric assertion below logs the gap so reviewers see it.
func TestCatalog_MatchesRegisteredHelpers(t *testing.T) {
	tpl := raymond.MustParse("")
	registerHelpers(tpl, &Options{}, map[string]any{}, nil)

	registered := map[string]bool{}
	for _, name := range tpl.HelperNames() {
		registered[name] = true
	}

	cataloged := map[string]bool{}
	for _, d := range builtinHelpers {
		cataloged[d.Name] = true
		if !registered[d.Name] {
			t.Errorf("cataloged helper %q is not registered on the runtime template", d.Name)
		}
	}

	var missing []string
	for name := range registered {
		if !cataloged[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("registered helpers missing from catalog: %v", missing)
	}
}

func TestCatalog_ReturnsCopy(t *testing.T) {
	a := Catalog()
	if len(a) == 0 {
		t.Fatal("Catalog returned empty slice")
	}
	a[0].Name = "MUTATED"
	b := Catalog()
	if b[0].Name == "MUTATED" {
		t.Errorf("Catalog leaked the backing slice - callers can mutate the registry")
	}
}

func TestCatalog_EveryEntryHasRequiredFields(t *testing.T) {
	for i, d := range builtinHelpers {
		if d.Name == "" {
			t.Errorf("entry %d: empty Name", i)
		}
		if d.Signature == "" {
			t.Errorf("entry %d (%q): empty Signature", i, d.Name)
		}
		if d.Description == "" {
			t.Errorf("entry %d (%q): empty Description", i, d.Name)
		}
		if d.Example == "" {
			t.Errorf("entry %d (%q): empty Example", i, d.Name)
		}
		if d.Category == "" {
			t.Errorf("entry %d (%q): empty Category", i, d.Name)
		}
		// Signature should contain the name so the panel renders are
		// self-consistent (user sees "yamlList" and the signature
		// starts with `{{yamlList`).
		if !strings.Contains(d.Signature, d.Name) {
			t.Errorf("entry %d (%q): signature %q does not mention the helper name",
				i, d.Name, d.Signature)
		}
	}
}

func TestCatalog_NoDuplicateNames(t *testing.T) {
	seen := map[string]int{}
	for i, d := range builtinHelpers {
		if prev, ok := seen[d.Name]; ok {
			t.Errorf("duplicate entry for %q at indexes %d and %d", d.Name, prev, i)
		}
		seen[d.Name] = i
	}
}
