package codeformatter

// formatLua is a placeholder for a proper Lua formatter. Today it
// applies the generic tidy pass (trailing whitespace, line endings,
// trailing newline). A real reformat needs a Lua parser + pretty
// printer - gopher-lua's AST is available, but writing the printer is
// out of scope for the initial cut. The frontend was previously using
// lua-fmt (npm); leaving the Lua tab pointing at this backend keeps
// the surface uniform so a future upgrade is a single-file swap.
func formatLua(src string) (string, error) {
	return tidy(src), nil
}
