package template

import "testing"

func TestListItemText(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{"plain", "plain"},
		{map[string]any{"text": "child", "indent": float64(1)}, "child"},
		{map[string]any{"indent": float64(2)}, ""},
		{float64(42), "42"},
		{nil, ""},
	}
	for _, c := range cases {
		if got := ListItemText(c.in); got != c.want {
			t.Errorf("ListItemText(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestListItemIndent(t *testing.T) {
	cases := []struct {
		in   any
		want int
	}{
		{"plain", 0},
		{map[string]any{"text": "a", "indent": float64(2)}, 2},
		{map[string]any{"text": "a"}, 0},
		{map[string]any{"text": "a", "indent": float64(-3)}, 0},
		{map[string]any{"text": "a", "indent": float64(999)}, MaxListIndent},
		{map[string]any{"text": "a", "indent": 3}, 3},
	}
	for _, c := range cases {
		if got := ListItemIndent(c.in); got != c.want {
			t.Errorf("ListItemIndent(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}
