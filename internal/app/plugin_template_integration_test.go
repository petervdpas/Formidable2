package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/plugin"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

const heldInvoiceYAML = `name: Invoice
fields:
  - key: number
    type: text
    label: Number
  - key: customer
    type: api-client
    label: Customer
    client_id: themis
    resource: contacts
`

// templatePlugin writes a plugin that holds invoice.yaml and wires it to a real template manager on a temp context.
func templatePlugin(t *testing.T) (*plugin.Manager, *template.Manager) {
	t.Helper()
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	tplM := template.NewManager(sys, "templates", nil)

	pluginsDir := filepath.Join(root, "plugins")
	dir := filepath.Join(pluginsDir, "invoicer")
	for name, body := range map[string]string{
		"plugin.json": `{"manifest_version": 1, "id": "invoicer", "name": "Invoicer", "version": "0.1.0",
			"commands": [{"id": "setup", "label": "Setup"}]}`,
		"main.lua": `
function setup(ctx)
  local first = formidable.template.install("invoice.yaml")
  local second = formidable.template.install("invoice.yaml")
  return { first = first, second = second }
end`,
		"templates/invoice.yaml": heldInvoiceYAML,
	} {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pluginM := plugin.NewManager(plugin.ManagerDeps{
		PluginsDir: pluginsDir,
		Editor:     system.NewManager(t.TempDir(), nil),
		Templates:  pluginTemplateInstaller{tpl: tplM},
	})
	if err := pluginM.Refresh(); err != nil {
		t.Fatal(err)
	}
	return pluginM, tplM
}

func TestPluginTemplate_HeldTemplateIsCreatedOnRequestAndUsable(t *testing.T) {
	pluginM, tplM := templatePlugin(t)
	if tplM.Exists("invoice.yaml") {
		t.Fatal("a refresh created the template")
	}
	rows, err := pluginM.ListShippedTemplates("invoicer")
	if err != nil || len(rows) != 1 || rows[0].Name != "Invoice" || rows[0].Fields != 2 || rows[0].Installed {
		t.Fatalf("rows = %+v, err = %v", rows, err)
	}

	rows, err = pluginM.CreateShippedTemplate("invoicer", "invoice.yaml")
	if err != nil || !rows[0].Installed {
		t.Fatalf("CreateShippedTemplate: rows = %+v, err = %v", rows, err)
	}
	got, err := tplM.LoadTemplate("invoice.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Invoice" || len(got.Fields) != 2 || got.Fields[1].ClientID != "themis" || got.Fields[1].Resource != "contacts" {
		t.Errorf("created template = %+v", got)
	}
}

func TestPluginTemplate_ScriptInstallIsCreateOnly(t *testing.T) {
	pluginM, tplM := templatePlugin(t)
	res, err := pluginM.Run("invoicer", "setup", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := res.Value.(map[string]any)
	// The run creates held templates before the script starts, so both script calls find it there.
	if got["first"] != false || got["second"] != false || !tplM.Exists("invoice.yaml") {
		t.Errorf("result = %v", got)
	}
}

func TestPluginTemplate_AnEditedTemplateSurvivesANewerCopyInThePlugin(t *testing.T) {
	pluginM, tplM := templatePlugin(t)
	if _, err := pluginM.CreateShippedTemplate("invoicer", "invoice.yaml"); err != nil {
		t.Fatal(err)
	}
	mine, _ := tplM.LoadTemplate("invoice.yaml")
	edited := *mine
	edited.Name = "Invoice, as I changed it"
	if err := tplM.SaveTemplate("invoice.yaml", &edited); err != nil {
		t.Fatal(err)
	}

	newer := strings.Replace(heldInvoiceYAML, "name: Invoice", "name: Invoice v2", 1)
	if _, err := pluginM.SaveShippedTemplate("invoicer", "invoice.yaml", newer); err != nil {
		t.Fatal(err)
	}
	if _, err := pluginM.CreateShippedTemplate("invoicer", "invoice.yaml"); err != nil {
		t.Fatal(err)
	}
	if _, err := pluginM.Run("invoicer", "setup", nil); err != nil {
		t.Fatal(err)
	}
	if after, _ := tplM.LoadTemplate("invoice.yaml"); after.Name != "Invoice, as I changed it" {
		t.Errorf("the user's template was replaced: name = %q", after.Name)
	}

	// The plugin's improved version is there for the taking, and only then.
	rows, err := pluginM.ListShippedTemplates("invoicer")
	if err != nil || !rows[0].Installed || !rows[0].Differs {
		t.Fatalf("rows = %+v, err = %v", rows, err)
	}
	rows, err = pluginM.ReplaceShippedTemplate("invoicer", "invoice.yaml")
	if err != nil || rows[0].Differs {
		t.Fatalf("ReplaceShippedTemplate: rows = %+v, err = %v", rows, err)
	}
	if after, _ := tplM.LoadTemplate("invoice.yaml"); after.Name != "Invoice v2" || after.Fields[1].ClientID != "themis" {
		t.Errorf("after the replace: %+v", after)
	}
}

func TestPluginTemplate_CopyFromTheContextIntoThePluginAndRefuseABadOne(t *testing.T) {
	pluginM, tplM := templatePlugin(t)
	if err := tplM.CreateFromYAML("offer.yaml", []byte("name: Offer\nfields:\n  - key: title\n    type: text\n    label: Title\n")); err != nil {
		t.Fatal(err)
	}
	rows, err := pluginM.AddShippedTemplateFrom("invoicer", "offer.yaml")
	if err != nil {
		t.Fatalf("AddShippedTemplateFrom: %v", err)
	}
	if len(rows) != 2 || rows[1].File != "offer.yaml" || rows[1].Name != "Offer" || !rows[1].Installed {
		t.Errorf("rows = %+v", rows)
	}

	dup := "name: Bad\nfields:\n  - key: a\n    type: text\n    label: A\n  - key: a\n    type: text\n    label: B\n"
	if _, err := pluginM.SaveShippedTemplate("invoicer", "bad.yaml", dup); err == nil {
		t.Error("a template that fails validation was stored in the plugin")
	}
	if _, err := pluginM.SaveShippedTemplate("invoicer", "bad.yaml", "\t: ["); err == nil {
		t.Error("text that is not YAML was stored in the plugin")
	}
}
