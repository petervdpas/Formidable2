package codeformatter

// formatLua is a placeholder: it applies only the generic tidy pass. A real
// reformat needs a Lua parser + pretty printer. Pointing the Lua tab here
// keeps the surface uniform so a future upgrade is a single-file swap.
func formatLua(src string) (string, error) {
	return tidy(src), nil
}
