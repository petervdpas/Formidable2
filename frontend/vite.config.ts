import { defineConfig, type Plugin } from "vite";
import vue from "@vitejs/plugin-vue";
import wails from "@wailsio/runtime/plugins/vite";
import { resolve } from "node:path";
import { writeFileSync } from "node:fs";

// Vite's watcher only watches files inside the project root by default.
// Our `frontend/src/styles/index.css` `@import`s an upstream CSS file
// from outside (`internal/modules/render/assets/formidable-prose.css`,
// the same file Go `//go:embed`s into the wiki HTTP server's output).
// Without this plugin, edits to that upstream file silently miss HMR
// and the slideout / wiki preview keep serving stale CSS until the dev
// server is fully restarted. The plugin tells chokidar about the
// extra path and triggers a full reload when it changes.
// Vite's default `emptyOutDir: true` wipes `frontend/dist/` clean on
// every build, including the tracked `.gitkeep` placeholder that keeps
// the directory present on fresh checkouts. Without that file, CI's
// `go test ./...` errors out at `//go:embed all:frontend/dist` before
// any test runs. Recreating the marker (with explanatory text) at
// `closeBundle` ensures every build leaves dist/ in a committable
// state.
function ensureGitkeep(): Plugin {
  const marker = resolve(__dirname, "dist/.gitkeep");
  const body =
    "# Do not delete.\n" +
    "#\n" +
    "# `main.go` uses `//go:embed all:frontend/dist` — the embed pattern\n" +
    "# requires this directory to exist at compile time, even on a fresh\n" +
    "# checkout before `task build:frontend` has populated it. CI tests\n" +
    "# fail with \"no matching files found\" without this placeholder.\n" +
    "#\n" +
    "# Recreated automatically by the `ensureGitkeep` Vite plugin after\n" +
    "# every build (Vite's emptyOutDir wipes the directory clean).\n";
  return {
    name: "formidable:ensure-gitkeep",
    apply: "build",
    closeBundle() {
      writeFileSync(marker, body);
    },
  };
}

function watchProseCSS(): Plugin {
  const externalCss = resolve(
    __dirname,
    "../internal/modules/render/assets/formidable-prose.css",
  );
  return {
    name: "formidable:watch-prose-css",
    configureServer(server) {
      server.watcher.add(externalCss);
      server.watcher.on("change", (file) => {
        if (file === externalCss) {
          server.ws.send({ type: "full-reload", path: "*" });
        }
      });
    },
  };
}

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue(), wails("./bindings"), watchProseCSS(), ensureGitkeep()],
  server: {
    host: "127.0.0.1",
    strictPort: true,
    // Allow imports from the parent (repo root) so frontend stylesheets
    // can pull from a single source of truth shared with Go embed —
    // e.g. internal/modules/render/assets/formidable-prose.css.
    fs: { allow: [".."] },
  },
  build: {
    // CodeMirror 6 alone is ~620KB minified — that's a known fixed
    // cost for shipping a code editor. With manualChunks isolating it,
    // the rest of the app is well under the 500KB default. Bumping
    // the warn threshold so it only fires if some other chunk grows
    // pathologically.
    chunkSizeWarningLimit: 750,
    rollupOptions: {
      // @vueuse/core (pulled in transitively by VueDatePicker) ships
      // /* #__PURE__ */ annotations in spots Rollup can't parse —
      // harmless but noisy. Pass them through silently; surface every
      // other warning untouched.
      onwarn(warning, warn) {
        if (
          warning.code === "INVALID_ANNOTATION" &&
          (warning.id ?? "").includes("@vueuse/core")
        ) {
          return;
        }
        warn(warning);
      },
      output: {
        // Pre-split heavy vendors so each lib gets its own chunk.
        // Dev rebuilds stay fast (only the chunks whose source changed
        // are re-emitted) and the browser/WebView can cache vendor
        // chunks across releases. Order doesn't matter; chunk names
        // are matched by import path.
        manualChunks: {
          vue: ["vue", "vue-i18n"],
          codemirror: [
            "vue-codemirror",
            "@codemirror/view",
            "@codemirror/state",
            "@codemirror/lang-markdown",
            "@codemirror/lang-yaml",
            "@codemirror/theme-one-dark",
          ],
          mdEditor: ["md-editor-v3"],
          fontawesome: ["@fortawesome/fontawesome-free"],
          dnd: ["vuedraggable"],
          datepicker: ["@vuepic/vue-datepicker", "date-fns"],
        },
      },
    },
  },
});
