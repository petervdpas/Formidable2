package codeformatter

import (
	"strings"
	"testing"
)

// pdfishSchemas mirrors a subset of pdf.Schemas() for these tests so
// the codeformatter package stays import-free of pdf. Real wiring is
// in app.go where pdf.Schemas() is passed at construction.
var pdfishSchemas = Schemas{
	"cover": {
		"template": true, "title": true, "subtitle": true,
		"author": true, "authorTitle": true, "organization": true,
		"date": true, "version": true, "logo": true,
		"documentType": true, "documentID": true,
		"description": true, "clientName": true, "projectName": true,
		"department": true,
	},
	"toc": {
		"title": true, "minDepth": true, "maxDepth": true,
	},
	"footer": {
		"position": true, "showPageNumber": true, "date": true,
		"status": true, "text": true, "documentID": true,
	},
}

func TestRepair_NestsFlatPDFFrontmatter(t *testing.T) {
	in := `---
cover:
template: classic
title: Auto-generated Report
subtitle: FCDM Entities
author: Formidable Generator
toc:
title: Contents
minDepth: 1
maxDepth: 3
footer:
position: center
showPageNumber: true
text: Formidable
documentID: ""
style: ""
keywords: [fcdm, entities]
---

## body
`
	out, err := NewManager(pdfishSchemas).Format("markdown", in)
	if err != nil {
		t.Fatal(err)
	}

	checks := []string{
		"cover:\n  template: classic\n  title: Auto-generated Report\n  subtitle: FCDM Entities\n  author: Formidable Generator",
		"toc:\n  title: Contents\n  minDepth: 1\n  maxDepth: 3",
		"footer:\n  position: center\n  showPageNumber: true\n  text: Formidable\n  documentID: \"\"",
	}
	for _, c := range checks {
		if !strings.Contains(out, c) {
			t.Errorf("missing block:\n%s\n\ngot:\n%s", c, out)
		}
	}

	if !strings.Contains(out, "\nstyle: \"\"\n") {
		t.Errorf("top-level style: should NOT be nested under footer:\n%s", out)
	}
	if !strings.Contains(out, "\nkeywords:") {
		t.Errorf("top-level keywords: missing:\n%s", out)
	}

	if !strings.Contains(out, "\n## body\n") {
		t.Errorf("body text altered or missing:\n%s", out)
	}
}

func TestRepair_LeavesAlreadyNestedAlone(t *testing.T) {
	in := `---
cover:
  template: classic
  title: Hello
---
`
	out, err := NewManager(pdfishSchemas).Format("markdown", in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "cover:\n  template: classic\n  title: Hello") {
		t.Errorf("nested input should pass through:\n%s", out)
	}
	if strings.Contains(out, "    template:") {
		t.Errorf("double-indented — repair pass ran on already-nested input:\n%s", out)
	}
}

func TestRepair_NilSchemas_NoOp(t *testing.T) {
	in := `---
cover:
template: classic
title: Hello
---
`
	out, err := NewManager(nil).Format("markdown", in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "  template:") {
		t.Errorf("repair ran with nil schemas:\n%s", out)
	}
}

func TestRepair_UnknownParentPassesThrough(t *testing.T) {
	in := `---
mystuff:
foo: bar
baz: qux
---
`
	out, err := NewManager(pdfishSchemas).Format("markdown", in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "  foo:") {
		t.Errorf("unknown parent should not trigger repair:\n%s", out)
	}
}
