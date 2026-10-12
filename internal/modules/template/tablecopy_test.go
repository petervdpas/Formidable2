package template

import (
	"reflect"
	"testing"
)

func tbl(key string, cols ...string) Field {
	opts := make([]any, 0, len(cols))
	for _, c := range cols {
		opts = append(opts, map[string]any{"value": c, "label": c})
	}
	return Field{Key: key, Type: "table", Label: "L-" + key, Options: opts}
}

func TestTableCopySources_OtherRootTablesSharingAColumn(t *testing.T) {
	fields := []Field{
		tbl("old", "a", "b", "x"),
		tbl("new", "a", "b", "c"),
		tbl("unrelated", "p", "q"),
		{Key: "L", Type: "loopstart"}, tbl("inner", "a"), {Key: "L", Type: "loopstop"},
		{Key: "t", Type: "text"},
	}
	want := []ItemField{{Key: "old", Label: "L-old"}}
	if got := TableCopySources(fields, "new"); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for _, target := range []string{"inner", "t", "ghost", ""} {
		if got := TableCopySources(fields, target); len(got) != 0 {
			t.Errorf("target %q: got %+v, want none", target, got)
		}
	}
}

func TestTableColumnMap_TargetIndexToSourceIndex(t *testing.T) {
	got := TableColumnMap(tbl("src", "x", "b", "a"), tbl("dst", "a", "b", "c"))
	if !reflect.DeepEqual(got, []int{2, 1, -1}) {
		t.Fatalf("got %v", got)
	}
	if got := TableColumnMap(tbl("src", "p"), tbl("dst", "a")); !reflect.DeepEqual(got, []int{-1}) {
		t.Fatalf("no shared column: %v", got)
	}
}
