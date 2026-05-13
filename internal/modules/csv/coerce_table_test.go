package csv

import (
	"reflect"
	"testing"
)

func TestCoerceTableRows_MixedTypes(t *testing.T) {
	cols := []TableColumn{
		{Type: "string"},
		{Type: "number"},
		{Type: "bool"},
		{Type: "date"},
		{Type: "dropdown", Choices: opts(
			[2]string{"uk", "United Kingdom"},
			[2]string{"us", "United States"},
		)},
	}
	rows := [][]string{
		{"Abbey Road", "3", "true", "2026-04-08", "United Kingdom"},
		{"Empire State", "350", "FALSE", "1931-05-01", "us"},
	}
	got := CoerceTableRows(cols, rows)
	want := [][]any{
		{"Abbey Road", float64(3), true, "2026-04-08", "uk"},
		{"Empire State", float64(350), false, "1931-05-01", "us"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CoerceTableRows mismatch:\n got=%#v\nwant=%#v", got, want)
	}
}

func TestCoerceTableRows_ShortRowPaddedWithEmpties(t *testing.T) {
	cols := []TableColumn{
		{Type: "string"},
		{Type: "number"},
		{Type: "bool"},
	}
	rows := [][]string{
		{"only-one-cell"},
	}
	got := CoerceTableRows(cols, rows)
	want := [][]any{
		{"only-one-cell", float64(0), false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("short-row padding wrong:\n got=%#v\nwant=%#v", got, want)
	}
}

func TestCoerceTableRows_LongRowTruncatedToColumns(t *testing.T) {
	cols := []TableColumn{
		{Type: "string"},
	}
	rows := [][]string{
		{"keep", "drop", "drop2"},
	}
	got := CoerceTableRows(cols, rows)
	want := [][]any{
		{"keep"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("long-row truncation wrong:\n got=%#v\nwant=%#v", got, want)
	}
}

func TestCoerceTableRows_DropdownLabelOrValueMatch(t *testing.T) {
	cols := []TableColumn{
		{Type: "dropdown", Choices: opts(
			[2]string{"uk", "United Kingdom"},
			[2]string{"us", "United States"},
		)},
	}
	rows := [][]string{
		{"United Kingdom"},
		{"us"},
		{"Atlantis"},
	}
	got := CoerceTableRows(cols, rows)
	want := [][]any{
		{"uk"},
		{"us"},
		{"Atlantis"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dropdown match wrong:\n got=%#v\nwant=%#v", got, want)
	}
}

func TestCoerceTableRows_EmptyInputs(t *testing.T) {
	if got := CoerceTableRows(nil, nil); len(got) != 0 {
		t.Fatalf("nil/nil should yield empty result, got=%#v", got)
	}
	cols := []TableColumn{{Type: "string"}, {Type: "number"}}
	got := CoerceTableRows(cols, [][]string{{}})
	want := [][]any{{"", float64(0)}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("empty row should pad with empties:\n got=%#v\nwant=%#v", got, want)
	}
}
