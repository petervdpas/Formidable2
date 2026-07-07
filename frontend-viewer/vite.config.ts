import { defineConfig, type Plugin } from "vite";
import vue from "@vitejs/plugin-vue";
import { resolve } from "node:path";
import { writeFileSync } from "node:fs";

// The Go binary does `//go:embed all:dist`, which needs dist/ to exist at
// compile time even on a fresh checkout. Vite's emptyOutDir wipes it (including
// the tracked .gitkeep) on every build, so recreate the marker afterwards.
function ensureGitkeep(): Plugin {
  const marker = resolve(__dirname, "dist/.gitkeep");
  return {
    name: "viewer:ensure-gitkeep",
    apply: "build",
    closeBundle() {
      writeFileSync(marker, "# keep dist present for //go:embed before first build\n");
    },
  };
}

// Wails serves its runtime at /wails/runtime.js but does not inject the script.
// Add it to the built HTML here, bypassing Rollup's module resolution (an
// absolute runtime-served URL it cannot and should not bundle).
function wailsRuntime(): Plugin {
  return {
    name: "viewer:wails-runtime",
    transformIndexHtml() {
      return [
        { tag: "script", attrs: { type: "module", src: "/wails/runtime.js" }, injectTo: "head" },
      ];
    },
  };
}

export default defineConfig({
  // Relative base so the embedded assets resolve under the Wails asset scheme.
  base: "./",
  plugins: [vue(), wailsRuntime(), ensureGitkeep()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
