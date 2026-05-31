package template

func boolPtr(b bool) *bool { return &b }
func intPtr(i int) *int    { return &i }

// basicTemplate is the seed used by SeedBasicIfEmpty.
func basicTemplate() *Template {
	return &Template{
		Name:     "Basic Form",
		Filename: basicYAMLName,
		Fields: []Field{
			{Key: "basic_form_test", Label: "Test", Type: "text", Default: "Default value", TwoColumn: true},
			{Key: "check", Label: "Check", Type: "boolean", TwoColumn: true},
			{
				Key: "basic_form_dropdown", Label: "Dropdown", Type: "dropdown",
				Default: "R", TwoColumn: true,
				Options: []any{
					map[string]any{"value": "L", "label": "Left"},
					map[string]any{"value": "R", "label": "Right"},
				},
			},
			{
				Key: "basic_form_multichoice", Label: "Multiple Choice", Type: "multioption",
				TwoColumn: true,
				Options: []any{
					map[string]any{"value": "A", "label": "Option A"},
					map[string]any{"value": "B", "label": "Option B"},
					map[string]any{"value": "C", "label": "Option C"},
				},
			},
			{
				Key: "basic_form_radio", Label: "Radio", Type: "radio",
				Default: "DOG", TwoColumn: true,
				Options: []any{
					map[string]any{"value": "CAT", "label": "Cat"},
					map[string]any{"value": "DOG", "label": "Dog"},
					map[string]any{"value": "BIRD", "label": "Bird"},
				},
			},
			{Key: "basic_form_mline", Label: "Mline", Type: "textarea", Default: "A whole lot of prefab text..."},
			{Key: "basic_form_numpy", Label: "Numpy", Type: "number", Default: "17"},
			{Key: "basic_form_bday", Label: "Birthday", Type: "date", Default: "1968-12-23"},
			{Key: "basic_form_listy", Label: "Listy", Type: "list"},
			{
				Key: "basic_form_datable", Label: "Table", Type: "table",
				Options: []any{
					map[string]any{"value": "col1", "label": "Column 1"},
					map[string]any{"value": "col2", "label": "Column 2"},
					map[string]any{"value": "col3", "label": "Column 3"},
				},
			},
		},
	}
}

var (
	_ = boolPtr
	_ = intPtr
)
