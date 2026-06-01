package config

// Numeric setting bounds enforced on load. The frontend's input min/max
// is the first line of defence; these clamps are the backend's safety
// net for user.json edited by hand, copied between versions, or written
// by future external tooling.
const (
	ToastTimeoutMin     = 2
	ToastTimeoutMax     = 15
	ToastTimeoutDefault = 5

	// DecimalPrecision: how many decimals chart percentage labels show.
	// 0 = whole numbers (the historical look).
	DecimalPrecisionMin     = 0
	DecimalPrecisionMax     = 3
	DecimalPrecisionDefault = 0
)

// defaultConfig returns a Config populated with the same defaults as
// `Formidable/schemas/config.schema.js`. Every field is explicit so
// downstream zero-value drift can't silently change behavior.
func defaultConfig() Config {
	return Config{
		ProfileName:          "Default Profile",
		Theme:                "light",
		ShowPasteButtons:     true,
		ShowSortButtons:      true,
		ShowDedupButtons:     true,
		UseExpressions:       false,
		ShowMetaSection:      true,
		IoCollectionOnly:     false,
		LoopStateCollapsed:   false,
		FieldStateCollapsed:  false,
		FontSize:             14,
		DevelopmentEnable:    false,
		LoggingEnabled:       false,
		EnablePlugins:        false,
		EnableFullTextSearch: false,
		UpdateCheck:          false,
		ContextMode:          "template",
		ContextRibbon:        "templates",
		ContextFolder:        "./Examples",
		SelectedTemplate:     "basic.yaml",
		SelectedDataFile:     "",
		AuthorName:           "unknown",
		AuthorEmail:          "unknown@example.com",
		Language:             "en",
		RemoteBackend:        "none",
		Git:                  GitConfig{},
		Gigot:                GigotConfig{},
		EnableInternalServer: false,
		InternalServerPort:   8383,
		WindowBounds: WindowBounds{
			Width:  1024,
			Height: 800,
		},
		SidebarWidth:     280,
		ToastTimeout:     ToastTimeoutDefault,
		DecimalPrecision: DecimalPrecisionDefault,
		StatusButtons: StatusButtons{
			Reloader:   true,
			Charpicker: true,
			Gitquick:   false,
			Gigotload:  false,
			Language:   true,
		},
		History: History{
			Enabled: true,
			Persist: false,
			MaxSize: 20,
			Stack:   []string{},
			Index:   -1,
		},
	}
}

func defaultBootConfig() BootConfig {
	return BootConfig{ActiveProfile: "user.json"}
}
