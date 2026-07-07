// applyTheme reflects the config theme on the shell. "light" sets a data-theme
// the stylesheet overrides against; "dark"/"system" use the dark default.
export function applyTheme(theme: string): void {
  const root = document.documentElement;
  if (theme === "light") {
    root.dataset.theme = "light";
  } else {
    delete root.dataset.theme;
  }
}
