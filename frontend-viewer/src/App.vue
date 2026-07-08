<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref } from "vue";
import { Events } from "@wailsio/runtime";
import { api, BundleChangedEvent, type BundleInfo, type OpenResult } from "./api";
import { bundleZoom, reportError } from "./state";
import { applyTheme } from "./theme";
import HomeScreen from "./components/HomeScreen.vue";
import SettingsDialog from "./components/SettingsDialog.vue";
import UnlockDialog from "./components/UnlockDialog.vue";

const emptyBundle: BundleInfo = {
  loaded: false,
  name: "",
  title: "",
  description: "",
  author: "",
  created: "",
  encrypted: false,
  hasData: false,
};
const bundle = ref<BundleInfo>({ ...emptyBundle });

// The unlock prompt for an encrypted pack. retry re-runs the same open with a
// password (path- or bytes-based), so the flow is uniform across dialog,
// recents, and drops.
type Retry = (password: string) => Promise<OpenResult>;
type UnlockPoke = { path?: string; name?: string; title?: string; description?: string; wrong?: boolean };
const unlock = ref<{
  show: boolean;
  name: string;
  title: string;
  description: string;
  wrong: boolean;
  busy: boolean;
  retry: Retry;
}>({ show: false, name: "", title: "", description: "", wrong: false, busy: false, retry: async () => ({ info: emptyBundle, needsPassword: false, wrongPassword: false, path: "" }) });
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

// drive handles an open result: load and switch view, or raise the unlock
// prompt for an encrypted pack. retry re-runs the same open with a password.
async function drive(res: OpenResult, retry: Retry): Promise<void> {
  if (res.info.loaded) {
    unlock.value.show = false;
    homeKey.value++; // recents changed; refresh them if the user returns home
    await refresh();
    return;
  }
  if (res.needsPassword || res.wrongPassword) {
    unlock.value = {
      show: true,
      name: res.info.name,
      title: res.info.title,
      description: res.info.description,
      wrong: res.wrongPassword,
      busy: false,
      retry,
    };
  }
}

async function openDialog(): Promise<void> {
  try {
    const res = await api.openDialog();
    await drive(res, (pw) => api.openPath(res.path, pw));
  } catch (e) {
    reportError(e);
  }
}

async function openRecent(path: string): Promise<void> {
  try {
    const res = await api.openPath(path, "");
    await drive(res, (pw) => api.openPath(path, pw));
  } catch (e) {
    reportError(e);
  }
}

async function submitUnlock(password: string): Promise<void> {
  unlock.value.busy = true;
  try {
    const retry = unlock.value.retry;
    const res = await retry(password);
    await drive(res, retry);
  } catch (e) {
    reportError(e);
  } finally {
    unlock.value.busy = false;
  }
}

function cancelUnlock(): void {
  unlock.value.show = false;
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
  if (!file || !file.name.toLowerCase().endsWith(".bundle")) return;
  try {
    const b64 = bufToBase64(await file.arrayBuffer());
    const res = await api.openBytes(file.name, b64, "");
    await drive(res, (pw) => api.openBytes(file.name, b64, pw));
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
  // Claim an argv / "open with" path and run it through the normal flow, so an
  // encrypted pack prompts for its password here rather than opening blank.
  try {
    const pending = await api.takePendingOpen();
    if (pending) await openRecent(pending);
  } catch (e) {
    reportError(e);
  }
  // Let the backend trigger the view switch after a native file drop directly,
  // so it flows through Vue's own refresh (not a full webview reload).
  (window as unknown as { __viewerRefresh?: () => void }).__viewerRefresh = () => {
    void refresh();
  };
  // The native drop handler pokes this for an encrypted pack: open the unlock
  // prompt seeded with the manifest, retrying the open by path with a password.
  (window as unknown as { __viewerUnlock?: (p: UnlockPoke) => void }).__viewerUnlock = (p) => {
    unlock.value = {
      show: true,
      name: p?.name ?? "",
      title: p?.title ?? "",
      description: p?.description ?? "",
      wrong: !!p?.wrong,
      busy: false,
      retry: (pw) => api.openPath(p?.path ?? "", pw),
    };
  };
  off = Events.On(BundleChangedEvent, () => {
    void refresh();
  });
});
onBeforeUnmount(() => {
  off?.();
  delete (window as unknown as { __viewerRefresh?: () => void }).__viewerRefresh;
  delete (window as unknown as { __viewerUnlock?: unknown }).__viewerUnlock;
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
      @open="openDialog"
      @open-recent="openRecent"
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

    <UnlockDialog
      v-if="unlock.show"
      :name="unlock.name"
      :title="unlock.title"
      :description="unlock.description"
      :wrong="unlock.wrong"
      :busy="unlock.busy"
      @submit="submitUnlock"
      @cancel="cancelUnlock"
    />
  </div>
</template>
