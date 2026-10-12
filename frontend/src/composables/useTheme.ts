import { computed, watchEffect } from "vue";
import { useConfig } from "./useConfig";

export type ThemeId = "light" | "dark" | "purplish" | "ledger";

const VALID_THEMES: readonly ThemeId[] = ["light", "dark", "purplish", "ledger"];

// Themes with a light page. Code editors and diagram previews pick their own
// light or dark look from this, never from the theme's name.
const LIGHT_THEMES: readonly ThemeId[] = ["light", "ledger"];

function normalize(t: string | undefined): ThemeId {
  return (VALID_THEMES as readonly string[]).includes(t ?? "")
    ? (t as ThemeId)
    : "light";
}

const { config, update } = useConfig();

// Apply data-theme to <html> whenever config.theme changes.
watchEffect(() => {
  document.documentElement.dataset.theme = normalize(config.value?.theme);
});

export function useTheme() {
  return {
    theme: computed<ThemeId>(() => normalize(config.value?.theme)),
    isLight: computed<boolean>(() => LIGHT_THEMES.includes(normalize(config.value?.theme))),
    setTheme: (t: ThemeId) => update({ theme: t }),
    options: VALID_THEMES,
  };
}
