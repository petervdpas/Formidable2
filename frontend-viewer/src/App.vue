<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref } from "vue";
import { Events } from "@wailsio/runtime";
import { api, BundleChangedEvent, type BundleInfo } from "./api";
import { bundleZoom, reportError } from "./state";
import { applyTheme } from "./theme";
import HomeScreen from "./components/HomeScreen.vue";
import SettingsDialog from "./components/SettingsDialog.vue";

const bundle = ref<BundleInfo>({ loaded: false, name: "" });
const view = ref<"home" | "bundle">("home");
const showSettings = ref(false);
// Bumped to force the iframe to reload when the bundle swaps.
const frameKey = ref(0);
// The loopback URL the bundle is served from (real http:// origin so it renders
// inside the sub-frame). Falls back to the in-app mount until fetched.
const bundleUrl = ref("/bundle/");
// Bumped to remount the home screen so its recents reload (e.g. after clear).
const homeKey = ref(0);
// Drag overlay so a file dropped anywhere (including over the iframe) lands on
// the shell instead of the cross-origin bundle frame.
const dragging = ref(false);
let dragDepth = 0;

async function refresh(): Promise<void> {
  try {
    bundle.value = await api.current();
    if (bundle.value.loaded) {
      frameKey.value++;
      view.value = "bundle";
    }
  } catch (e) {
    reportError(e);
  }
}

function bufToBase64(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf);
  let binary = "";
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunk));
  }
  return btoa(binary);
}

function hasFiles(e: DragEvent): boolean {
  return Array.from(e.dataTransfer?.types ?? []).includes("Files");
}

function onDragEnter(e: DragEvent): void {
  if (!hasFiles(e)) return;
  e.preventDefault();
  dragDepth++;
  dragging.value = true;
}
function onDragOver(e: DragEvent): void {
  if (!hasFiles(e)) return;
  e.preventDefault(); // required to allow a drop
}
function onDragLeave(): void {
  dragDepth = Math.max(0, dragDepth - 1);
  if (dragDepth === 0) dragging.value = false;
}
async function onDrop(e: DragEvent): Promise<void> {
  e.preventDefault();
  dragDepth = 0;
  dragging.value = false;
  const file = e.dataTransfer?.files?.[0];
  if (!file || !file.name.toLowerCase().endsWith(".zip")) return;
  try {
    const b64 = bufToBase64(await file.arrayBuffer());
    const info = await api.openBytes(file.name, b64);
    if (info.loaded) await refresh();
  } catch (err) {
    reportError(err);
  }
}

let off: (() => void) | undefined;
onMounted(async () => {
  try {
    bundleUrl.value = await api.bundleURL();
    const cfg = await api.getConfig();
    applyTheme(cfg.theme);
    bundleZoom.value = cfg.default_zoom;
  } catch (e) {
    reportError(e);
  }
  await refresh();
  // Let the backend trigger the view switch after a native file drop directly,
  // so it flows through Vue's own refresh (not a full webview reload).
  (window as unknown as { __viewerRefresh?: () => void }).__viewerRefresh = () => {
    void refresh();
  };
  off = Events.On(BundleChangedEvent, () => {
    void refresh();
  });
});
onBeforeUnmount(() => {
  off?.();
  delete (window as unknown as { __viewerRefresh?: () => void }).__viewerRefresh;
});

function goHome(): void {
  view.value = "home";
}
function resume(): void {
  if (bundle.value.loaded) view.value = "bundle";
}
function closeSettings(): void {
  showSettings.value = false;
  homeKey.value++; // pick up any recents change (e.g. Clear)
}
</script>

<template>
  <div
    class="shell"
    @dragenter="onDragEnter"
    @dragover="onDragOver"
    @dragleave="onDragLeave"
    @drop="onDrop"
  >
    <HomeScreen
      v-if="view === 'home'"
      :key="homeKey"
      :current="bundle"
      @resume="resume"
      @opened="refresh"
      @open-settings="showSettings = true"
    />

    <div v-else class="frame-wrap">
      <iframe
        :key="frameKey"
        class="bundle-frame"
        :src="bundleUrl"
        :style="{ zoom: bundleZoom }"
        title="bundle"
      ></iframe>
      <div class="frame-controls">
        <button class="fc-btn" :title="$t('toolbar.home')" @click="goHome">
          <span class="fc-glyph">⌂</span>
        </button>
        <button class="fc-btn" :title="$t('toolbar.settings')" @click="showSettings = true">
          <span class="fc-glyph">⚙</span>
        </button>
      </div>
    </div>

    <div v-if="dragging" class="drop-overlay">
      {{ $t("home.drop_hint") }}
    </div>

    <SettingsDialog v-if="showSettings" @close="closeSettings" />
  </div>
</template>
