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
		ShowSortButtons:      false,
		ShowDedupButtons:     false,
		UseExpressions:       false,
		ShowMetaSection:      true,
		ShowCopyButton:       true,
		IoCollectionOnly:     false,
		LoopStateCollapsed:   false,
		FieldStateCollapsed:  false,
		FontSize:             14,
		UIFontSize:           UIFontSizeDefault,
		UIFontFamily:         "",
		HTMLPreviewZoom:      HTMLPreviewZoomDefault,
		DevelopmentEnable:    false,
		LoggingEnabled:       false,
		EnablePlugins:        false,
		EnableFullTextSearch: false,
		EnableRelationSync:   false,
		GraphLoopRows:        false,
		UpdateCheck:          false,
		ContextMode:          "template",
		ContextRibbon:        "templates",
		ContextFolder:        "./Examples",
		SelectedTemplate:     "basic.yaml",
		SelectedDataFile:     "",
		SavedQueryLocation:   "local",
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

// UI font sizes on offer, as the root em in pixels, in display order. The root
// the whole interface is drawn from, so the set is closed on purpose: an
// arbitrary value would let a profile land on a size no layout was checked at.
//
// This is a new key rather than the older `font_size`, which mirrors the
// Electron schema and was never wired to anything. A profile written before
// this existed has no `ui_font_size` at all, so it loads as 0 and snaps to the
// default below, which is the browser root every rem in the stylesheets was
// authored against. Reusing the dead key would instead have read its inert 14
// as a deliberate choice and shrunk every existing profile by an eighth.
var UIFontSizes = []int{12, 13, 14, 15, 16, 17, 18, 20}

// UIFontSizeDefault is the browser default root, which is what the app has
// always rendered at.
const UIFontSizeDefault = 16

// KnownUIFontSize reports whether px is one of the offered sizes.
func KnownUIFontSize(px int) bool {
	for _, s := range UIFontSizes {
		if s == px {
			return true
		}
	}
	return false
}

// Zoom levels for the HTML preview, as percentages in display order. The
// preview shows a rendered document rather than app chrome, so it zooms on its
// own: the reading size of a report has nothing to do with how big the
// surrounding UI should be. A closed set, for the same reason the font sizes
// are one.
var HTMLPreviewZooms = []int{75, 90, 100, 110, 125, 150, 175, 200}

// HTMLPreviewZoomDefault renders the document at its authored size.
const HTMLPreviewZoomDefault = 100

// KnownHTMLPreviewZoom reports whether pct is one of the offered levels.
func KnownHTMLPreviewZoom(pct int) bool {
	for _, z := range HTMLPreviewZooms {
		if z == pct {
			return true
		}
	}
	return false
}
