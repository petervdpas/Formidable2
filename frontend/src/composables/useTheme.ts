import { ref, watchEffect } from "vue";

export type ThemeId = "light" | "dark" | "purplish";

const theme = ref<ThemeId>("light");

watchEffect(() => {
  document.documentElement.dataset.theme = theme.value;
});

export function useTheme() {
  return {
    theme,
    setTheme: (t: ThemeId) => { theme.value = t; },
  };
}
