package codeformatter

import (
	"errors"
	"testing"
)

func TestFormat_JSON_IndentsAndKeepsOrderAndSpelling(t *testing.T) {
	src := `{"swagger":"2.0","info":{"title":"Pets","version":"1.0"},"big":9223372036854775807,"n":1.50,"tags":["b","a"],"empty":{},"esc":"aé\/b"}`
	want := `{
  "swagger": "2.0",
  "info": {
    "title": "Pets",
    "version": "1.0"
  },
  "big": 9223372036854775807,
  "n": 1.50,
  "tags": [
    "b",
    "a"
  ],
  "empty": {},
  "esc": "aé\/b"
}
`
	got, err := NewManager(nil).Format("json", src)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormat_JSON_IsIdempotent(t *testing.T) {
	m := NewManager(nil)
	once, err := m.Format("json", "  [1,{\"a\":null}]\n\n")
	if err != nil {
		t.Fatal(err)
	}
	twice, err := m.Format("json", once)
	if err != nil || twice != once {
		t.Errorf("second pass changed the text:\n%s\n%s (%v)", once, twice, err)
	}
}

func TestFormat_JSON_MalformedLeavesTheTextAlone(t *testing.T) {
	for _, src := range []string{`{"a": 1,}`, `{"a": 1} trailing`, `{'a': 1}`, `{"a"`} {
		got, err := NewManager(nil).Format("json", src)
		if !errors.Is(err, ErrMalformed) {
			t.Errorf("%q: err = %v, want ErrMalformed", src, err)
		}
		if got != src {
			t.Errorf("%q: text changed to %q", src, got)
		}
	}
}

func TestFormat_JSON_EmptyStaysEmpty(t *testing.T) {
	for _, src := range []string{"", "  \n"} {
		if got, err := NewManager(nil).Format("json", src); err != nil || got != src {
			t.Errorf("%q -> %q, %v", src, got, err)
		}
	}
}
