import { computed, watchEffect } from "vue";
import { useConfig } from "./useConfig";

export type ThemeId = "light" | "dark" | "purplish";

const VALID_THEMES: readonly ThemeId[] = ["light", "dark", "purplish"];

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
    setTheme: (t: ThemeId) => update({ theme: t }),
    options: VALID_THEMES,
  };
}
