package connection

import (
	"encoding/json"
	"testing"
)

func TestValidPointer(t *testing.T) {
	valid := []string{"", "/", "/a", "/a/b", "/a/0/b", "/~0", "/~1", "/a~0b~1c", "/ ", "/a b"}
	for _, p := range valid {
		if !ValidPointer(p) {
			t.Errorf("ValidPointer(%q) = false, want true", p)
		}
	}
	invalid := []string{"a", "a/b", "/~", "/~2", "/a/~x", "~0"}
	for _, p := range invalid {
		if ValidPointer(p) {
			t.Errorf("ValidPointer(%q) = true, want false", p)
		}
	}
}

func TestResolvePointer(t *testing.T) {
	var doc any
	src := `{
	  "data": {"items": [{"id": 7, "name": "Ada", "tags": ["x"]}, {"id": 8}]},
	  "a/b": "slash",
	  "m~n": "tilde",
	  "nil": null,
	  "flag": true
	}`
	if err := json.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		ptr  string
		want string
		ok   bool
	}{
		{"", "", true},
		{"/data/items/0/name", "Ada", true},
		{"/data/items/0/id", "7", true},
		{"/data/items/1/id", "8", true},
		{"/data/items/0/tags/0", "x", true},
		{"/a~1b", "slash", true},
		{"/m~0n", "tilde", true},
		{"/flag", "true", true},
		{"/nil", "", true},
		{"/data/items/9/id", "", false},
		{"/data/items/-1", "", false},
		{"/data/items/x", "", false},
		{"/missing", "", false},
		{"/data/items/0/name/deeper", "", false},
		{"/flag/nope", "", false},
	}
	for _, tc := range cases {
		got, ok := ResolvePointer(doc, tc.ptr)
		if ok != tc.ok {
			t.Errorf("ResolvePointer(%q) ok = %v, want %v", tc.ptr, ok, tc.ok)
			continue
		}
		if ok && tc.ptr != "" && got != tc.want {
			t.Errorf("ResolvePointer(%q) = %q, want %q", tc.ptr, got, tc.want)
		}
	}
}

func TestResolvePointer_WholeDocumentString(t *testing.T) {
	got, ok := ResolvePointer("plain-id", "")
	if !ok || got != "plain-id" {
		t.Fatalf("empty pointer over a scalar = %q/%v, want plain-id/true", got, ok)
	}
}

func TestResolvePointer_NumbersDoNotGainExponents(t *testing.T) {
	var doc any
	if err := json.Unmarshal([]byte(`{"id": 1234567890123}`), &doc); err != nil {
		t.Fatal(err)
	}
	got, ok := ResolvePointer(doc, "/id")
	if !ok || got != "1234567890123" {
		t.Fatalf("got %q/%v, want 1234567890123/true", got, ok)
	}
}

func TestResolvePointer_InvalidPointerFails(t *testing.T) {
	if _, ok := ResolvePointer(map[string]any{"a": "b"}, "a"); ok {
		t.Fatal("a pointer without a leading slash must not resolve")
	}
}
