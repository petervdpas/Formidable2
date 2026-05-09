package template

func assignLevelScopes(fields []Field) []Field {
	out := make([]Field, len(fields))
	copy(out, fields)
	scope := 0
	for i := range out {
		switch out[i].Type {
		case "loopstart":
			out[i].LevelScope = scope
			scope++
		case "loopstop":
			scope--
			if scope < 0 {
				scope = 0
			}
			out[i].LevelScope = scope
		default:
			out[i].LevelScope = scope
		}
	}
	return out
}

func expressionItemLevelScopeErrors(fields []Field) []ValidationError {
	var errs []ValidationError
	for i := range fields {
		f := fields[i]
		if !f.ExpressionItem || f.LevelScope == 0 {
			continue
		}
		ff := f
		errs = append(errs, ValidationError{
			Type:    "expression-item-non-root",
			Field:   &ff,
			Index:   i,
			Key:     f.Key,
			Detail:  map[string]any{"levelScope": f.LevelScope},
			Message: "Expression fields must sit at the root level (level_scope 0); " + f.Key + " is at level " + itoa(f.LevelScope),
		})
	}
	return errs
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := false
	if n < 0 {
		negative = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
