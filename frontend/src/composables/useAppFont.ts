// The app's typeface and root size are profile settings, owned by Go and
// written onto the document root here, the same shape useTheme uses for the
// theme. Every rem in the stylesheets resolves against --font-size-root and
// every sans-serif surface reads --font-stack, so these two writes redraw the
// whole window; --font-mono is deliberately untouched, so code editors and
// monospace fields keep their own face.

import { computed, ref, watch } from "vue";
import { useConfig } from "./useConfig";
import {
  Service as FontsSvc,
  type UIFont,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/fonts";
import { Service as ConfigSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/config";

const FONT_FACE_STYLE_ID = "formidable-app-font-faces";

const sizes = ref<number[]>([]);
const fonts = ref<UIFont[]>([]);
const { config, update } = useConfig();

function applySize(px: number | undefined) {
  if (!px) return; // absent until the config lands; the stylesheet default holds
  document.documentElement.style.setProperty("--font-size-root", `${px}px`);
}

function applyStack(stack: string) {
  if (!stack) return;
  document.documentElement.style.setProperty("--font-stack", stack);
}

// The @font-face rules for uploaded fonts, each inlined as a data: URI by the
// backend. Injected once so a user's own font is available to the app chrome,
// not only to a rendered deck.
async function installFontFaces() {
  try {
    const css = await FontsSvc.FontFaceCSS();
    if (!css) return;
    let el = document.getElementById(FONT_FACE_STYLE_ID) as HTMLStyleElement | null;
    if (!el) {
      el = document.createElement("style");
      el.id = FONT_FACE_STYLE_ID;
      document.head.appendChild(el);
    }
    el.textContent = css;
  } catch {
    // No faces means uploaded families fall back to the system stack, which is
    // exactly what their resolved stack already lists behind them.
  }
}

// The stored id resolves to a CSS stack on the backend, so an id whose font
// file is gone lands on the system stack rather than on a family the platform
// cannot find.
async function applyFamily(id: string | undefined) {
  try {
    const font = await FontsSvc.ResolveUIFont(id ?? "");
    applyStack(font.stack);
  } catch {
    // leave whatever the stylesheet declares
  }
}

let started = false;

// loadAppFont installs the faces, fetches the backend-owned choice lists, and
// starts following the profile. Call once at boot; a profile switch flows in
// through config on its own.
export async function loadAppFont(): Promise<void> {
  if (started) return;
  started = true;

  await installFontFaces();
  try {
    [sizes.value, fonts.value] = await Promise.all([
      ConfigSvc.UIFontSizes(),
      FontsSvc.ListUIFonts(),
    ]);
  } catch {
    sizes.value = [];
    fonts.value = [];
  }

  watch(() => config.value?.ui_font_size, applySize, { immediate: true });
  watch(() => config.value?.ui_font_family, applyFamily, { immediate: true });
}

export function useAppFont() {
  return {
    size: computed(() => config.value?.ui_font_size ?? 16),
    family: computed(() => config.value?.ui_font_family ?? ""),
    sizes: computed(() => sizes.value),
    fonts: computed(() => fonts.value),
    setSize: (px: number) => update({ ui_font_size: px }),
    setFamily: (id: string) => update({ ui_font_family: id }),
    // A font added or removed in the Fonts panel changes the pickable set and
    // the faces, so that panel calls this rather than waiting for a restart.
    refresh: async () => {
      await installFontFaces();
      try {
        fonts.value = await FontsSvc.ListUIFonts();
      } catch {
        // keep the list we have
      }
      await applyFamily(config.value?.ui_font_family);
    },
  };
}
