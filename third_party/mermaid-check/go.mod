// Vendored fork of github.com/sammcj/mermaid-check (cache tag go-mermaid@v0.0.4).
// Library packages only: cmd/ and its fatih/color dependency are dropped, so this
// is pure-stdlib. Upstream declared `go 1.26.2` with no actual 1.26 feature use
// (no version gates anywhere in the source); floor lowered to 1.25 so it builds on
// our toolchain. Wired via a replace directive in the root go.mod, same pattern as
// third_party/raymond.
module github.com/sammcj/mermaid-check

go 1.25
