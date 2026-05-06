import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import wails from "@wailsio/runtime/plugins/vite";

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue(), wails("./bindings")],
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
          easymde: ["easymde"],
          fontawesome: ["@fortawesome/fontawesome-free"],
          dnd: ["vuedraggable"],
          datepicker: ["@vuepic/vue-datepicker", "date-fns"],
        },
      },
    },
  },
});
