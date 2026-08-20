<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useTheme } from "../../composables/useTheme";
import type { SpecDocument } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/connection";

const { t } = useI18n();
const { theme } = useTheme();

const props = defineProps<{
  document: SpecDocument | null;
  /** The vendored renderer, fetched once per session by the parent. */
  assets: { css: string; bundle: string; preset: string } | null;
  loading: boolean;
}>();

// The page runs in an iframe rather than in the app's own document: Swagger UI
// ships a full reset stylesheet that would repaint every panel around it.
const blobURL = ref("");

function revoke(): void {
  if (blobURL.value) {
    URL.revokeObjectURL(blobURL.value);
    blobURL.value = "";
  }
}

// Dark mode is a filter rather than a theme: swagger-ui ships no dark build,
// and hand-restyling several hundred selectors would drift from the vendored
// copy on every upgrade. Images are inverted back so logos stay right.
const darkCSS = `
  html { filter: invert(1) hue-rotate(180deg); background: #1e1e1e; }
  img, svg, .swagger-ui .info__extdocs { filter: invert(1) hue-rotate(180deg); }
`;

function buildPage(): string {
  const spec = props.document?.json ?? "{}";
  const a = props.assets;
  if (!a) return "";
  return [
    "<!doctype html>",
    '<html lang="en"><head><meta charset="utf-8">',
    "<style>", a.css, "</style>",
    "<style>body{margin:0;background:#fff;}</style>",
    theme.value === "light" ? "" : "<style>" + darkCSS + "</style>",
    '</head><body><div id="swagger-ui"></div>',
    "<script>", a.bundle, "<\/script>",
    "<script>", a.preset, "<\/script>",
    "<script>",
    // The spec goes in as an object, so the first paint needs no fetch at all.
    "var spec = ", spec, ";",
    // Expanding an operation makes swagger-ui resolve that subtree, and its
    // resolver fetches `baseDoc`, which it derives from the page URL when no
    // url is given. This page is a blob in a sandboxed iframe, so that fetch
    // is a cross-origin read of an opaque origin: it fails, the resolve never
    // settles, and the operation spins forever. A data: URL is fetchable from
    // any origin, so pointing baseDoc at the same document fixes the resolve
    // without loosening the sandbox.
    "var docURL = 'data:application/json,' + encodeURIComponent(JSON.stringify(spec));",
    "window.ui = SwaggerUIBundle({",
    "  spec: spec,",
    "  url: docURL,",
    '  dom_id: "#swagger-ui",',
    "  deepLinking: false,",
    "  presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],",
    "  plugins: [SwaggerUIBundle.plugins.DownloadUrl],",
    '  layout: "BaseLayout",',
    // Nothing here can reach a remote: the console in the Try tab is the
    // supported way to call, and it goes through the invoker with the
    // client's own auth and base URL.
    "  supportedSubmitMethods: []",
    "});",
    "<\/script>",
    "</body></html>",
  ].join("\n");
}

function render(): void {
  revoke();
  if (!props.document || !props.assets) return;
  const html = buildPage();
  if (!html) return;
  blobURL.value = URL.createObjectURL(new Blob([html], { type: "text/html" }));
}

watch(() => [props.document, props.assets, theme.value] as const, render, { immediate: true });

onBeforeUnmount(revoke);
</script>

<template>
  <p v-if="loading" class="muted small">{{ t('apiclients.document.loading') }}</p>
  <p v-else-if="!document" class="muted small">{{ t('apiclients.document.empty') }}</p>
  <p v-else-if="!assets" class="muted small">{{ t('apiclients.document.no_renderer') }}</p>
  <iframe
    v-else
    class="apiclients-swagger"
    :src="blobURL"
    :title="document.file"
    sandbox="allow-scripts"
  ></iframe>
</template>
