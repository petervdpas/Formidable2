package mermaid

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initMermaidScenario,
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

type mermaidWorld struct {
	source string
	result Result
}

func initMermaidScenario(ctx *godog.ScenarioContext) {
	w := &mermaidWorld{}

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		w.source = ""
		w.result = Result{}
		return ctx, nil
	})

	ctx.Step(`^the Mermaid source:$`, func(doc *godog.DocString) error {
		w.source = doc.Content
		return nil
	})

	ctx.Step(`^empty Mermaid source$`, func() error {
		w.source = ""
		return nil
	})

	ctx.Step(`^whitespace-only Mermaid source$`, func() error {
		w.source = "   \n\t\n  "
		return nil
	})

	ctx.Step(`^I validate it$`, func() error {
		w.result = Validate(w.source)
		return nil
	})

	ctx.Step(`^validation succeeds$`, func() error {
		if !w.result.OK {
			return fmt.Errorf("expected OK, got errors: %+v", w.result.Errors)
		}
		return nil
	})

	ctx.Step(`^validation fails$`, func() error {
		if w.result.OK {
			return fmt.Errorf("expected failure, got OK (type %q)", w.result.DiagramType)
		}
		return nil
	})

	ctx.Step(`^the diagram type is "([^"]*)"$`, func(want string) error {
		if w.result.DiagramType != want {
			return fmt.Errorf("diagram type = %q, want %q", w.result.DiagramType, want)
		}
		return nil
	})

	ctx.Step(`^the diagram type is empty$`, func() error {
		if w.result.DiagramType != "" {
			return fmt.Errorf("diagram type = %q, want empty", w.result.DiagramType)
		}
		return nil
	})

	ctx.Step(`^there are no issues$`, func() error {
		if len(w.result.Errors) != 0 {
			return fmt.Errorf("expected no issues, got %+v", w.result.Errors)
		}
		return nil
	})

	ctx.Step(`^there (?:is|are) (\d+) issues?$`, func(n int) error {
		if len(w.result.Errors) != n {
			return fmt.Errorf("issue count = %d, want %d: %+v", len(w.result.Errors), n, w.result.Errors)
		}
		return nil
	})

	ctx.Step(`^issue (\d+) has severity "([^"]*)"$`, func(idx int, sev string) error {
		e, err := w.issue(idx)
		if err != nil {
			return err
		}
		if e.Severity != sev {
			return fmt.Errorf("issue %d severity = %q, want %q", idx, e.Severity, sev)
		}
		return nil
	})

	ctx.Step(`^issue (\d+) has code "([^"]*)"$`, func(idx int, code string) error {
		e, err := w.issue(idx)
		if err != nil {
			return err
		}
		if e.Code != code {
			return fmt.Errorf("issue %d code = %q, want %q", idx, e.Code, code)
		}
		return nil
	})

	ctx.Step(`^issue (\d+) is at line (\d+)$`, func(idx, line int) error {
		e, err := w.issue(idx)
		if err != nil {
			return err
		}
		if e.Line != line {
			return fmt.Errorf("issue %d line = %d, want %d", idx, e.Line, line)
		}
		return nil
	})

	ctx.Step(`^issue (\d+) message contains "([^"]*)"$`, func(idx int, sub string) error {
		e, err := w.issue(idx)
		if err != nil {
			return err
		}
		if !strings.Contains(e.Message, sub) {
			return fmt.Errorf("issue %d message %q does not contain %q", idx, e.Message, sub)
		}
		return nil
	})
}

func (w *mermaidWorld) issue(idx int) (Issue, error) {
	if idx < 1 || idx > len(w.result.Errors) {
		return Issue{}, fmt.Errorf("issue %d out of range (have %d)", idx, len(w.result.Errors))
	}
	return w.result.Errors[idx-1], nil
}
