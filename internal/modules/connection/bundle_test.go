package connection

import (
	"errors"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSetBundleSettings_WritesAMinimalSettingsFile(t *testing.T) {
	out, err := SetBundleSettings(nil, "shop", BundleSettings{BaseURL: "  https://acc.shop.example.com/v1 "})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "id: shop\nbase_url: https://acc.shop.example.com/v1\n" {
		t.Errorf("out = %q", out)
	}
	got, err := ReadBundleSettings(out)
	if err != nil || got.BaseURL != "https://acc.shop.example.com/v1" || got.Custom {
		t.Errorf("settings = %+v, %v", got, err)
	}
}

func TestSetBundleSettings_ClearingLeavesNothingToShip(t *testing.T) {
	first, _ := SetBundleSettings(nil, "shop", BundleSettings{BaseURL: "https://shop.example.com"})
	out, err := SetBundleSettings(first, "shop", BundleSettings{BaseURL: ""})
	if err != nil || out != nil {
		t.Errorf("out = %q, err = %v; want nil so the file is removed", out, err)
	}
	if out, err := SetBundleSettings(nil, "shop", BundleSettings{BaseURL: ""}); err != nil || out != nil {
		t.Errorf("empty on empty: out = %q, err = %v", out, err)
	}
}

func TestSetBundleSettings_KeepsAHandWrittenDefinition(t *testing.T) {
	c := validConn()
	def, _ := yaml.Marshal(c)

	out, err := SetBundleSettings(def, "crm-prod", BundleSettings{Name: c.Name, BaseURL: "https://acc.crm.example.com/v2"})
	if err != nil {
		t.Fatal(err)
	}
	var got Connection
	if err := yaml.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got.BaseURL != "https://acc.crm.example.com/v2" || got.Name != c.Name || len(got.Resources) != 1 || got.Resources[0].List.Params["tenant"] != "acme" {
		t.Errorf("definition = %+v", got)
	}
	settings, _ := ReadBundleSettings(out)
	if !settings.Custom {
		t.Error("a definition with bindings must read as custom")
	}

	cleared, err := SetBundleSettings(out, "crm-prod", BundleSettings{BaseURL: ""})
	if err != nil || cleared == nil {
		t.Fatalf("clearing the URL must keep the hand-written rest; out = %q, err = %v", cleared, err)
	}
}

func TestSetBundleSettings_Unhappy(t *testing.T) {
	for _, bad := range []string{"shop.example.com", "/v1", "ftp://shop.example.com", "https://"} {
		if _, err := SetBundleSettings(nil, "shop", BundleSettings{BaseURL: bad}); !errors.Is(err, ErrInvalidBaseURL) {
			t.Errorf("%q: err = %v, want ErrInvalidBaseURL", bad, err)
		}
	}
	if _, err := SetBundleSettings([]byte("\t: ["), "shop", BundleSettings{BaseURL: "https://shop.example.com"}); err == nil {
		t.Error("an unreadable definition was overwritten")
	}
	if _, err := SetBundleSettings([]byte("id: other\n"), "shop", BundleSettings{BaseURL: "https://shop.example.com"}); err == nil {
		t.Error("a definition for another id was accepted")
	}
}

func TestReadBundleSettings(t *testing.T) {
	if got, err := ReadBundleSettings(nil); err != nil || got != (BundleSettings{}) {
		t.Errorf("empty: %+v, %v", got, err)
	}
	if _, err := ReadBundleSettings([]byte("\t: [")); err == nil {
		t.Error("unreadable definition accepted")
	}
	if got, _ := ReadBundleSettings([]byte("id: shop\nname: Shop\n")); got.Custom || got.Name != "Shop" {
		t.Errorf("a name is a setting, not a hand-written definition: %+v", got)
	}
	if got, _ := ReadBundleSettings([]byte("id: shop\nauth:\n  kind: bearer\n")); !got.Custom {
		t.Error("a definition that sets auth is more than settings")
	}
}

func TestSetBundleSettings_NameAndBaseURLTogetherAndApart(t *testing.T) {
	out, err := SetBundleSettings(nil, "shop", BundleSettings{Name: "  My shop ", BaseURL: "https://shop.example.com"})
	if err != nil || string(out) != "id: shop\nname: My shop\nbase_url: https://shop.example.com\n" {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	out, err = SetBundleSettings(out, "shop", BundleSettings{Name: "My shop"})
	if err != nil || string(out) != "id: shop\nname: My shop\n" {
		t.Fatalf("name only: out = %q, err = %v", out, err)
	}
	if got, _ := ReadBundleSettings(out); got.Name != "My shop" || got.BaseURL != "" || got.Custom {
		t.Errorf("settings = %+v", got)
	}
	if out, err = SetBundleSettings(out, "shop", BundleSettings{}); err != nil || out != nil {
		t.Errorf("clearing both: out = %q, err = %v", out, err)
	}
}

func TestInstall_ShippedNameWinsOverTheDocumentTitle(t *testing.T) {
	def, _ := SetBundleSettings(nil, "shop", BundleSettings{Name: "Webshop (acceptance)"})
	m := NewManager(newMemFS(), nil)
	if _, err := m.Install("shop", readerOf(map[string]string{"shop.yaml": string(def), "specs/shop.json": derivableSpec})); err != nil {
		t.Fatalf("Install: %v", err)
	}
	c, _, _ := m.Get("shop")
	if c.Name != "Webshop (acceptance)" || len(c.Resources) != 1 || c.Auth.Kind != AuthBearer {
		t.Errorf("client = %+v", c)
	}
}
