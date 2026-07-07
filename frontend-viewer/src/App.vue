<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref } from "vue";
import { Events } from "@wailsio/runtime";
import { api, BundleChangedEvent, type BundleInfo } from "./api";
import { lastError, lastEvent, bundleZoom, clearError, noteEvent, reportError } from "./state";
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
  off = Events.On(BundleChangedEvent, () => {
    noteEvent(`${BundleChangedEvent} @ ${new Date().toLocaleTimeString()}`);
    void refresh();
  });
});
onBeforeUnmount(() => off?.());

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
  <div class="shell">
    <div v-if="lastError" class="err-banner">
      <span class="err-text">{{ lastError }}</span>
      <button class="err-close" @click="clearError">×</button>
    </div>

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

    <SettingsDialog v-if="showSettings" @close="closeSettings" />

    <div v-if="lastEvent" class="debug-line">evt: {{ lastEvent }}</div>
  </div>
</template>
