import { viewerTheme } from "./state";

// applyTheme reflects the config theme on the shell and records the resolved
// light/dark value (collapsing "system" via the OS preference) so it can be
// pushed to the bundle iframes. "light" sets a data-theme the stylesheet
// overrides against; dark is the shell's default (no data-theme).
export function applyTheme(theme: string): void {
  const root = document.documentElement;
  let resolved: "light" | "dark";
  if (theme === "light") resolved = "light";
  else if (theme === "dark") resolved = "dark";
  else resolved = window.matchMedia?.("(prefers-color-scheme: dark)").matches ? "dark" : "light";

  if (resolved === "light") root.dataset.theme = "light";
  else delete root.dataset.theme;
  viewerTheme.value = resolved;
}
