package csv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/petervdpas/formidable2/internal/modules/system"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initCsvScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}

type csvWorld struct {
	tmp        string
	sys        *system.Manager
	m          *Manager
	preview    PreviewResult
	previewErr error
	write      WriteResult
}

func initCsvScenario(ctx *godog.ScenarioContext) {
	w := &csvWorld{}

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		dir, err := os.MkdirTemp("", "csv-godog-")
		if err != nil {
			return ctx, err
		}
		w.tmp = dir
		w.sys = nil
		w.m = nil
		w.preview = PreviewResult{}
		w.previewErr = nil
		w.write = WriteResult{}
		return ctx, nil
	})

	ctx.After(func(ctx context.Context, sc *godog.Scenario, _ error) (context.Context, error) {
		if w.tmp != "" {
			_ = os.RemoveAll(w.tmp)
		}
		return ctx, nil
	})

	// ── Background ────────────────────────────────────────────────────

	ctx.Step(`^a system manager rooted at a temp directory$`, func() error {
		w.sys = system.NewManager(w.tmp, nil)
		return nil
	})

	ctx.Step(`^a csv manager wrapping that system$`, func() error {
		w.m = NewManager(w.sys, nil)
		return nil
	})

	// ── Givens ────────────────────────────────────────────────────────

	ctx.Step(`^the file "([^"]*)" with content "([^"]*)"$`, func(path, content string) error {
		decoded := decodeEscapes(content)
		return w.sys.SaveFile(path, decoded)
	})

	// Docstring variant — used when the content contains embedded quotes
	// or commas that don't survive the simple "([^"]*)" regex.
	ctx.Step(`^the file "([^"]*)" with the following content:$`, func(path string, body *godog.DocString) error {
		// Append a trailing newline to match the typical CSV-with-final-LF
		// convention from the JS source.
		content := body.Content
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		return w.sys.SaveFile(path, content)
	})

	// ── Whens ─────────────────────────────────────────────────────────

	ctx.Step(`^I preview "([^"]*)" with delimiter "([^"]*)"$`, func(path, delim string) error {
		w.preview, w.previewErr = w.m.Preview(path, delim)
		return nil
	})

	ctx.Step(`^I write "([^"]*)" with rows "([^"]*)" and delimiter "([^"]*)"$`, func(path, rowsSpec, delim string) error {
		rows := decodeRows(rowsSpec)
		w.write = w.m.Write(path, rows, delim)
		return nil
	})

	// Docstring variant — each line is one row, cells separated by the
	// pipe character `|`. Avoids embedded-quote quoting in the feature.
	ctx.Step(`^I write "([^"]*)" with the following rows and delimiter "([^"]*)":$`, func(path, delim string, body *godog.DocString) error {
		var rows [][]string
		for line := range strings.SplitSeq(body.Content, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			rows = append(rows, strings.Split(line, "|"))
		}
		w.write = w.m.Write(path, rows, delim)
		return nil
	})

	// ── Thens ─────────────────────────────────────────────────────────

	ctx.Step(`^the preview headers are "([^"]*)"$`, func(want string) error {
		got := strings.Join(w.preview.Headers, ",")
		if got != want {
			return fmt.Errorf("headers = %q, want %q", got, want)
		}
		return nil
	})

	ctx.Step(`^the preview headers count is (\d+)$`, func(want int) error {
		if len(w.preview.Headers) != want {
			return fmt.Errorf("headers len = %d, want %d", len(w.preview.Headers), want)
		}
		return nil
	})

	ctx.Step(`^the preview row count is (\d+)$`, func(want int) error {
		if w.preview.RowCount != want {
			return fmt.Errorf("rowCount = %d, want %d", w.preview.RowCount, want)
		}
		if len(w.preview.Rows) != want {
			return fmt.Errorf("len(rows) = %d, want %d", len(w.preview.Rows), want)
		}
		return nil
	})

	ctx.Step(`^the preview row (\d+) contains "([^"]*)"$`, func(idx int, want string) error {
		return assertRowContains(w, idx, decodeEscapes(want))
	})

	// Single-quoted variant for cases where the expected text contains
	// double quotes (e.g. checking that escaped quotes round-tripped).
	ctx.Step(`^the preview row (\d+) contains '([^']*)'$`, func(idx int, want string) error {
		return assertRowContains(w, idx, want)
	})

	ctx.Step(`^the preview returned an error$`, func() error {
		if w.previewErr == nil && w.preview.Error == "" {
			return fmt.Errorf("expected preview error, got %+v", w.preview)
		}
		return nil
	})

	ctx.Step(`^the write result is success$`, func() error {
		if !w.write.Success {
			return fmt.Errorf("expected success, got: %+v", w.write)
		}
		return nil
	})

	ctx.Step(`^the file "([^"]*)" exists$`, func(path string) error {
		if _, err := os.Stat(filepath.Join(w.tmp, path)); err != nil {
			return fmt.Errorf("expected %q to exist: %v", path, err)
		}
		return nil
	})

	ctx.Step(`^the file "([^"]*)" is empty$`, func(path string) error {
		info, err := os.Stat(filepath.Join(w.tmp, path))
		if err != nil {
			return err
		}
		if info.Size() != 0 {
			return fmt.Errorf("expected empty, got %d bytes", info.Size())
		}
		return nil
	})

	ctx.Step(`^the file "([^"]*)" has no carriage returns$`, func(path string) error {
		body, err := os.ReadFile(filepath.Join(w.tmp, path))
		if err != nil {
			return err
		}
		if strings.Contains(string(body), "\r") {
			return fmt.Errorf("file contains CR: %q", body)
		}
		return nil
	})
}

func assertRowContains(w *csvWorld, idx int, want string) error {
	if idx >= len(w.preview.Rows) {
		return fmt.Errorf("row %d out of range (len=%d)", idx, len(w.preview.Rows))
	}
	got := strings.Join(w.preview.Rows[idx], ",")
	if got != want {
		return fmt.Errorf("row %d = %q, want %q", idx, got, want)
	}
	return nil
}

// decodeEscapes turns literal `\n`, `\t`, `\"` sequences in feature-file
// strings into their actual byte values, so scenarios can express CSV
// content without doc-strings.
func decodeEscapes(s string) string {
	r := strings.NewReplacer(
		`\n`, "\n",
		`\t`, "\t",
		`\r`, "\r",
		`\"`, `"`,
	)
	return r.Replace(s)
}

// decodeRows parses a "row1|row2|row3" form into [][]string. Each row's
// fields are split on the literal delimiter expected for that scenario,
// minus any quoting — quotes inside a field (e.g. "Amsterdam, NL") are
// preserved as part of the cell text after stripping outer quotes.
//
// Empty input → nil rows.
func decodeRows(spec string) [][]string {
	if strings.TrimSpace(spec) == "" {
		return nil
	}
	rows := strings.Split(spec, "|")
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, splitRowSpec(r))
	}
	return out
}

// splitRowSpec splits a row-spec on commas OR semicolons, honoring
// double-quoted segments (which preserve the delimiter inside).
func splitRowSpec(row string) []string {
	var fields []string
	var cur strings.Builder
	inQuotes := false
	for i := 0; i < len(row); i++ {
		c := row[i]
		switch {
		case c == '"':
			inQuotes = !inQuotes
		case (c == ',' || c == ';') && !inQuotes:
			fields = append(fields, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	fields = append(fields, cur.String())
	return fields
}
