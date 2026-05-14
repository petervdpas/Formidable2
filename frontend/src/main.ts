import "./styles/index.css";
import { createApp } from "vue";
import { Events } from "@wailsio/runtime";
import App from "./App.vue";
import { i18n } from "./i18n";
import { useI18nLoader } from "./composables/useI18nLoader";
import { ensureFieldTypesLoaded } from "./types/field-types";
import { installConsoleBridge } from "./utils/consoleBridge";
import * as PdfSvc from "../bindings/github.com/petervdpas/formidable2/internal/modules/pdf/service";

// Re-publish console.* through the backend slog pipeline so frontend
// lines appear in formidable.log + the Information→Logging live tail.
// Devtools keeps receiving the originals unchanged.
installConsoleBridge();
console.info("spa: console bridge installed");

// Dev-only debug pokes — exposed unconditionally for now so they're
// reachable from devtools regardless of vite mode. Strip the whole
// block before shipping a production build.
(window as unknown as { __pdf: typeof PdfSvc }).__pdf = PdfSvc;

// Kick off bundle load before mount so first paint already has the
// active locale. The composable resolves itself on the second boot.
useI18nLoader();

// Field-type registry is the Go single-source-of-truth (Service.FieldTypes).
// Pre-load so the Templates editor's Type dropdown + showRow() see a
// populated cache on first render. Fire-and-forget — the registry ref
// is reactive, so a component that catches the empty window re-renders
// when the load resolves.
void ensureFieldTypesLoaded();

createApp(App).use(i18n).mount("#app");

// Tell Go the SPA is mounted so it can dismiss the splash window and
// reveal the (currently hidden) main window. Sent on the next frame so
// the first paint has actually happened.
requestAnimationFrame(() => {
  void Events.Emit("spa:ready");
});
