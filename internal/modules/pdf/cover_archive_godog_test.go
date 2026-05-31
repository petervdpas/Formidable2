package pdf

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// coverArchiveWorld carries state across the cover-archive godog
// scenarios. It owns its own pdfWorld-shaped pair of memFS instances:
// the primary `mem` and a `fresh` (used only by the round-trip
// scenario to simulate a teammate's clean machine).
type coverArchiveWorld struct {
	svc       *Service
	mgr       *Manager
	mem       *memFS
	fresh     *memFS
	freshSvc  *Service
	freshMgr  *Manager
	actionErr error

	// Snapshots set by When-steps; the corresponding Then-steps read them.
	exportRes ExportCoverArchiveResult
	importRes ImportCoverArchiveResult
	zipBytes  []byte

	// Captured to test "did not clobber" assertions.
	originalBodies map[string]string
}

func (w *coverArchiveWorld) reset() { *w = coverArchiveWorld{originalBodies: map[string]string{}} }

func initCoverArchiveScenario(ctx *godog.ScenarioContext) {
	w := &coverArchiveWorld{originalBodies: map[string]string{}}

	ctx.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w.reset()
		return ctx, nil
	})

	ctx.Step(`^the scaffolded cover library is materialised on disk$`, func() error {
		w.mem = newMemFS()
		w.mgr = &Manager{
			log:    slog.Default(),
			store:  &store{fs: w.mem, log: slog.Default()},
			status: Status{Source: SourceUnset},
			nowFn:  func() time.Time { return time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC) },
		}
		w.svc = NewService(w.mgr)
		return scaffoldCovers(w.mem, slog.Default())
	})

	ctx.Step(`^a user cover "([^"]*)" exists with image refs "([^"]*)"$`, func(name, refsCSV string) error {
		refs := splitAndTrim(refsCSV)
		var imgs strings.Builder
		for _, r := range refs {
			imgs.WriteString(`<img src="`)
			imgs.WriteString(r)
			imgs.WriteString(`">`)
		}
		body := caCoverWithRefs(name, imgs.String())
		w.mem.files[onDiskCoversDir+"/"+name+".html"] = body
		w.originalBodies[name] = body
		return nil
	})

	ctx.Step(`^a user cover "([^"]*)" exists with no image refs$`, func(name string) error {
		body := caCoverWithRefs(name, "")
		w.mem.files[onDiskCoversDir+"/"+name+".html"] = body
		w.originalBodies[name] = body
		return nil
	})

	ctx.Step(`^an image "([^"]*)" exists on disk under the covers images dir$`, func(image string) error {
		w.mem.files[onDiskCoversDir+"/"+coverImagesSubdir+"/"+image] = "fake-image-bytes-" + image
		return nil
	})

	ctx.Step(`^I ExportCoverArchive cover "([^"]*)" to "([^"]*)"$`, func(name, zipPath string) error {
		w.exportRes, w.actionErr = w.svc.ExportCoverArchive(name, zipPath)
		return nil
	})

	ctx.Step(`^the export archive zip contains "([^"]*)"$`, func(entry string) error {
		raw, err := w.mem.LoadFile(w.exportRes.ZipPath)
		if err != nil {
			return fmt.Errorf("read zip %q: %w", w.exportRes.ZipPath, err)
		}
		zr, err := zip.NewReader(bytes.NewReader([]byte(raw)), int64(len(raw)))
		if err != nil {
			return fmt.Errorf("parse zip: %w", err)
		}
		for _, f := range zr.File {
			if f.Name == entry {
				return nil
			}
		}
		var names []string
		for _, f := range zr.File {
			names = append(names, f.Name)
		}
		return fmt.Errorf("entry %q not in zip (got %v)", entry, names)
	})

	ctx.Step(`^the export archive reports no missing images$`, func() error {
		if len(w.exportRes.MissingImages) > 0 {
			return fmt.Errorf("got missing images %v, want none", w.exportRes.MissingImages)
		}
		return nil
	})

	ctx.Step(`^the export archive reports missing image "([^"]*)"$`, func(image string) error {
		for _, m := range w.exportRes.MissingImages {
			if m == image {
				return nil
			}
		}
		return fmt.Errorf("missing %v, want to include %q", w.exportRes.MissingImages, image)
	})

	ctx.Step(`^the export archive reports (\d+) bundled images$`, func(want int) error {
		if got := len(w.exportRes.Images); got != want {
			return fmt.Errorf("got %d bundled, want %d (%v)", got, want, w.exportRes.Images)
		}
		return nil
	})

	ctx.Step(`^the service action returned ErrCoverArchiveNotFound$`, func() error {
		if !errors.Is(w.actionErr, ErrCoverArchiveNotFound) {
			return fmt.Errorf("got %v, want ErrCoverArchiveNotFound", w.actionErr)
		}
		return nil
	})

	ctx.Step(`^the service action returned ErrCoverArchiveInvalid$`, func() error {
		if !errors.Is(w.actionErr, ErrCoverArchiveInvalid) {
			return fmt.Errorf("got %v, want ErrCoverArchiveInvalid", w.actionErr)
		}
		return nil
	})

	ctx.Step(`^the service action returned ErrCoverArchiveExists$`, func() error {
		if !errors.Is(w.actionErr, ErrCoverArchiveExists) {
			return fmt.Errorf("got %v, want ErrCoverArchiveExists", w.actionErr)
		}
		return nil
	})

	ctx.Step(`^the service action returned ErrCoverArchiveTraversal$`, func() error {
		if !errors.Is(w.actionErr, ErrCoverArchiveTraversal) {
			return fmt.Errorf("got %v, want ErrCoverArchiveTraversal", w.actionErr)
		}
		return nil
	})

	ctx.Step(`^the service action returned ErrCoverInvalid$`, func() error {
		if !errors.Is(w.actionErr, ErrCoverInvalid) {
			return fmt.Errorf("got %v, want ErrCoverInvalid", w.actionErr)
		}
		return nil
	})

	// ── Import ────────────────────────────────────────────────────

	ctx.Step(`^a cover archive at "([^"]*)" with cover "([^"]*)" and image "([^"]*)"$`, func(zipPath, coverName, image string) error {
		entries := map[string]string{
			coverName + ".html": caCoverWithRefs(coverName, `<img src="`+image+`">`),
			"images/" + image:   "image-bytes-" + image,
		}
		w.mem.files[zipPath] = string(buildZip(entries))
		return nil
	})

	ctx.Step(`^I ImportCoverArchive from "([^"]*)" with overwrite=(true|false)$`, func(zipPath, ov string) error {
		w.importRes, w.actionErr = w.svc.ImportCoverArchive(zipPath, ov == "true")
		return nil
	})

	ctx.Step(`^the imported cover name is "([^"]*)"$`, func(want string) error {
		if w.importRes.Name != want {
			return fmt.Errorf("got %q, want %q", w.importRes.Name, want)
		}
		return nil
	})

	ctx.Step(`^the cover "([^"]*)" exists on disk$`, func(name string) error {
		if !w.mem.FileExists(onDiskCoversDir + "/" + name + ".html") {
			return fmt.Errorf("cover %q not on disk", name)
		}
		return nil
	})

	ctx.Step(`^the cover image "([^"]*)" exists on disk$`, func(image string) error {
		if !w.mem.FileExists(onDiskCoversDir + "/" + coverImagesSubdir + "/" + image) {
			return fmt.Errorf("image %q not on disk", image)
		}
		return nil
	})

	ctx.Step(`^the import was not flagged as overwriting$`, func() error {
		if w.importRes.Overwritten {
			return fmt.Errorf("Overwritten = true, want false")
		}
		return nil
	})

	ctx.Step(`^the import was flagged as overwriting$`, func() error {
		if !w.importRes.Overwritten {
			return fmt.Errorf("Overwritten = false, want true")
		}
		return nil
	})

	ctx.Step(`^the cover "([^"]*)" on disk still matches its original body$`, func(name string) error {
		got := w.mem.files[onDiskCoversDir+"/"+name+".html"]
		want := w.originalBodies[name]
		if got != want {
			return fmt.Errorf("cover %q body diverged: got %q, want %q", name, got, want)
		}
		return nil
	})

	ctx.Step(`^the cover "([^"]*)" on disk no longer matches its original body$`, func(name string) error {
		got := w.mem.files[onDiskCoversDir+"/"+name+".html"]
		want := w.originalBodies[name]
		if got == want {
			return fmt.Errorf("cover %q body unchanged after overwrite", name)
		}
		return nil
	})

	ctx.Step(`^a malformed zip is on disk at "([^"]*)"$`, func(zipPath string) error {
		w.mem.files[zipPath] = "this-is-not-a-zip"
		return nil
	})

	ctx.Step(`^a zip at "([^"]*)" containing only entry "([^"]*)"$`, func(zipPath, entry string) error {
		w.mem.files[zipPath] = string(buildZip(map[string]string{entry: "x"}))
		return nil
	})

	ctx.Step(`^a zip at "([^"]*)" containing two cover html files$`, func(zipPath string) error {
		w.mem.files[zipPath] = string(buildZip(map[string]string{
			"a.html": caCoverWithRefs("a", ""),
			"b.html": caCoverWithRefs("b", ""),
		}))
		return nil
	})

	ctx.Step(`^a zip at "([^"]*)" with cover named "([^"]*)"$`, func(zipPath, name string) error {
		w.mem.files[zipPath] = string(buildZip(map[string]string{
			name + ".html": caCoverWithRefs(name, ""),
		}))
		return nil
	})

	ctx.Step(`^a zip at "([^"]*)" with a traversal entry$`, func(zipPath string) error {
		w.mem.files[zipPath] = string(buildZip(map[string]string{
			"team.html":        caCoverWithRefs("team", ""),
			"../../etc/passwd": "pwned",
		}))
		return nil
	})

	ctx.Step(`^no cover was materialised from the traversal zip$`, func() error {
		if w.mem.FileExists(onDiskCoversDir + "/team.html") {
			return fmt.Errorf("team.html materialised despite traversal in same zip")
		}
		return nil
	})

	ctx.Step(`^a zip at "([^"]*)" with an unexpected entry "([^"]*)"$`, func(zipPath, entry string) error {
		w.mem.files[zipPath] = string(buildZip(map[string]string{
			"team.html": caCoverWithRefs("team", ""),
			entry:       "x",
		}))
		return nil
	})

	ctx.Step(`^a zip at "([^"]*)" with cover html that fails validation$`, func(zipPath string) error {
		w.mem.files[zipPath] = string(buildZip(map[string]string{
			"team.html": `<div>no markers, no magic line</div>`,
		}))
		return nil
	})

	// ── Round-trip with fresh fs ──────────────────────────────────

	ctx.Step(`^I move the exported zip to a fresh fs at "([^"]*)"$`, func(zipPath string) error {
		raw, err := w.mem.LoadFile(w.exportRes.ZipPath)
		if err != nil {
			return err
		}
		w.fresh = newMemFS()
		w.freshMgr = &Manager{
			log:    slog.Default(),
			store:  &store{fs: w.fresh, log: slog.Default()},
			status: Status{Source: SourceUnset},
			nowFn:  func() time.Time { return time.Now().UTC() },
		}
		w.freshSvc = NewService(w.freshMgr)
		if err := scaffoldCovers(w.fresh, slog.Default()); err != nil {
			return err
		}
		w.fresh.files[zipPath] = raw
		return nil
	})

	ctx.Step(`^I ImportCoverArchive from "([^"]*)" with overwrite=(true|false) on the fresh fs$`, func(zipPath, ov string) error {
		w.importRes, w.actionErr = w.freshSvc.ImportCoverArchive(zipPath, ov == "true")
		return nil
	})

	ctx.Step(`^the cover "([^"]*)" exists on the fresh fs$`, func(name string) error {
		if !w.fresh.FileExists(onDiskCoversDir + "/" + name + ".html") {
			return fmt.Errorf("cover %q not on fresh fs", name)
		}
		return nil
	})

	ctx.Step(`^the cover image "([^"]*)" exists on the fresh fs$`, func(image string) error {
		if !w.fresh.FileExists(onDiskCoversDir + "/" + coverImagesSubdir + "/" + image) {
			return fmt.Errorf("image %q not on fresh fs", image)
		}
		return nil
	})
}

// ─── helpers ─────────────────────────────────────────────────────────

// caCoverWithRefs builds a minimal valid cover HTML with the requested
// refs HTML inlined. "ca" prefix for "cover-archive" to avoid clashing
// with helpers other test files may add.
func caCoverWithRefs(name, refsHTML string) string {
	return `<!--
  formidable-cover: 1
  name: ` + name + `
-->
<section class="cover" data-cover-start>` + refsHTML + `<h1>{{.Title}}</h1></section><span data-cover-end></span>`
}

func splitAndTrim(csv string) []string {
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func buildZip(entries map[string]string) []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for name, body := range entries {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(body))
	}
	_ = zw.Close()
	return buf.Bytes()
}
