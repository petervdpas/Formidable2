package render

import (
	"testing"
	"time"
)

func TestHelper_Today(t *testing.T) {
	orig := nowFn
	defer func() { nowFn = orig }()
	nowFn = func() time.Time {
		return time.Date(2026, 5, 17, 22, 30, 0, 0, time.UTC)
	}
	got := renderWithCtx(t, `{{today}}`, map[string]any{})
	if got != "2026-05-17" {
		t.Errorf("got %q, want %q", got, "2026-05-17")
	}
}

func TestHelper_Now_DefaultLayout(t *testing.T) {
	orig := nowFn
	defer func() { nowFn = orig }()
	nowFn = func() time.Time {
		return time.Date(2026, 5, 17, 22, 30, 45, 0, time.UTC)
	}
	got := renderWithCtx(t, `{{now}}`, map[string]any{})
	if got != "2026-05-17 22:30:45" {
		t.Errorf("got %q, want %q", got, "2026-05-17 22:30:45")
	}
}

func TestHelper_Now_CustomLayout(t *testing.T) {
	orig := nowFn
	defer func() { nowFn = orig }()
	nowFn = func() time.Time {
		return time.Date(2026, 5, 17, 22, 30, 0, 0, time.UTC)
	}
	got := renderWithCtx(t, `{{now "02-01-2006"}}`, map[string]any{})
	if got != "17-05-2026" {
		t.Errorf("got %q, want %q", got, "17-05-2026")
	}
}

func TestHelper_DateFormat_YMD(t *testing.T) {
	got := renderWithCtx(t, `{{dateFormat "2026-04-28" "02-01-2006"}}`, map[string]any{})
	if got != "28-04-2026" {
		t.Errorf("got %q, want %q", got, "28-04-2026")
	}
}

func TestHelper_DateFormat_RFC3339(t *testing.T) {
	got := renderWithCtx(t, `{{dateFormat "2026-04-28T10:15:00Z" "Mon, 02 Jan 2006"}}`, map[string]any{})
	if got != "Tue, 28 Apr 2026" {
		t.Errorf("got %q, want %q", got, "Tue, 28 Apr 2026")
	}
}

func TestHelper_DateFormat_Unparseable_Passthrough(t *testing.T) {
	got := renderWithCtx(t, `{{dateFormat "not-a-date" "02-01-2006"}}`, map[string]any{})
	if got != "not-a-date" {
		t.Errorf("got %q, want passthrough", got)
	}
}

func TestHelper_DateFormat_Empty(t *testing.T) {
	got := renderWithCtx(t, `{{dateFormat "" "02-01-2006"}}`, map[string]any{})
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestHelper_DateFormat_LocaleNL_LongDayMonth(t *testing.T) {
	// 2026-04-28 is a Tuesday.
	got := renderWithCtx(t, `{{dateFormat "2026-04-28" "Monday 2 January 2006" "nl"}}`, map[string]any{})
	if got != "dinsdag 28 april 2026" {
		t.Errorf("got %q, want %q", got, "dinsdag 28 april 2026")
	}
}

func TestHelper_DateFormat_LocaleNL_Short(t *testing.T) {
	got := renderWithCtx(t, `{{dateFormat "2026-04-28" "Mon, 02 Jan 2006" "nl"}}`, map[string]any{})
	if got != "di, 28 apr 2026" {
		t.Errorf("got %q, want %q", got, "di, 28 apr 2026")
	}
}

func TestHelper_DateFormat_LocaleDE(t *testing.T) {
	got := renderWithCtx(t, `{{dateFormat "2026-04-28" "Monday, 2. January 2006" "de"}}`, map[string]any{})
	if got != "Dienstag, 28. April 2026" {
		t.Errorf("got %q, want %q", got, "Dienstag, 28. April 2026")
	}
}

func TestHelper_DateFormat_LocaleFR(t *testing.T) {
	got := renderWithCtx(t, `{{dateFormat "2026-04-28" "Monday 2 January 2006" "fr"}}`, map[string]any{})
	if got != "mardi 28 avril 2026" {
		t.Errorf("got %q, want %q", got, "mardi 28 avril 2026")
	}
}

func TestHelper_DateFormat_UnknownLocale_Passthrough(t *testing.T) {
	got := renderWithCtx(t, `{{dateFormat "2026-04-28" "Mon, 02 Jan 2006" "xy"}}`, map[string]any{})
	if got != "Tue, 28 Apr 2026" {
		t.Errorf("got %q, want English passthrough", got)
	}
}

func TestHelper_Now_WithLocale(t *testing.T) {
	orig := nowFn
	defer func() { nowFn = orig }()
	nowFn = func() time.Time {
		return time.Date(2026, 5, 17, 22, 30, 0, 0, time.UTC)
	}
	got := renderWithCtx(t, `{{now "Mon, 02 Jan 2006" "nl"}}`, map[string]any{})
	if got != "zo, 17 mei 2026" {
		t.Errorf("got %q, want %q", got, "zo, 17 mei 2026")
	}
}

func TestHelper_DateFormat_NoNamesUnchangedByLocale(t *testing.T) {
	got := renderWithCtx(t, `{{dateFormat "2026-04-28" "02-01-2006" "nl"}}`, map[string]any{})
	if got != "28-04-2026" {
		t.Errorf("got %q, want %q", got, "28-04-2026")
	}
}
