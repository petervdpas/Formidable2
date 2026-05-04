package config

// defaultConfig returns a Config populated with the same defaults as
// `Formidable/schemas/config.schema.js`. Every field is explicit so
// downstream zero-value drift can't silently change behavior.
func defaultConfig() Config {
	return Config{
		ProfileName:          "Default Profile",
		Theme:                "light",
		ShowIconButtons:      false,
		ShowPasteButtons:     true,
		UseExpressions:       false,
		ShowMetaSection:      true,
		LoopStateCollapsed:   false,
		FieldStateCollapsed:  false,
		FontSize:             14,
		DevelopmentEnable:    false,
		LoggingEnabled:       false,
		EnablePlugins:        false,
		ContextMode:          "template",
		ContextFolder:        "./Examples",
		SelectedTemplate:     "basic.yaml",
		SelectedDataFile:     "",
		AuthorName:           "unknown",
		AuthorEmail:          "unknown@example.com",
		Language:             "en",
		EncryptionKey:        "",
		UseGit:               false,
		GitRoot:              "",
		GitBranch:            "",
		RemoteBackend:        "none",
		GigotBaseURL:         "",
		GigotRepoName:        "",
		GigotToken:           "",
		EnableInternalServer: false,
		InternalServerPort:   8383,
		WindowBounds: WindowBounds{
			Width:  1024,
			Height: 800,
		},
		TemplateSidebarWidth: 300,
		StorageSidebarWidth:  300,
		StatusButtons: StatusButtons{
			Reloader:   true,
			Charpicker: true,
			Gitquick:   false,
			Gigotload:  false,
		},
		History: History{
			Enabled: true,
			Persist: false,
			MaxSize: 20,
			Stack:   []any{},
			Index:   -1,
		},
	}
}

func defaultBootConfig() BootConfig {
	return BootConfig{ActiveProfile: "user.json"}
}
