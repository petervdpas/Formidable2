<script setup lang="ts">
import { onMounted, ref } from "vue";
import { api, type BundleInfo, type RecentInfo } from "../api";
import { reportError } from "../state";

defineProps<{ current: BundleInfo }>();
const emit = defineEmits<{
  resume: [];
  "open-settings": [];
  open: [];
  "open-recent": [path: string];
}>();

const recents = ref<RecentInfo[]>([]);

async function loadRecents(): Promise<void> {
  try {
    recents.value = await api.recents();
  } catch (e) {
    reportError(e);
  }
}
onMounted(loadRecents);

// The parent owns the open flow (it drives the password prompt for encrypted
// packs). The home screen only signals intent.
function openRecent(r: RecentInfo): void {
  if (!r.exists) return;
  emit("open-recent", r.path);
}
</script>

<template>
  <div class="home">
    <div class="home-inner">
      <img class="home-logo" src="/feather.svg" alt="" />
      <h1 class="home-title">Formidable Viewer</h1>
      <p class="home-hint">{{ $t("home.drop_hint") }}</p>

      <div class="home-actions">
        <button class="btn primary" @click="emit('open')">{{ $t("home.open_button") }}</button>
        <button
          v-if="current.loaded"
          class="btn"
          :title="current.name"
          @click="emit('resume')"
        >
          ▸ {{ $t("home.resume") }}
        </button>
        <button class="btn ghost" @click="emit('open-settings')">{{ $t("toolbar.settings") }}</button>
      </div>

      <div v-if="recents.length" class="recents">
        <div class="recents-title">{{ $t("home.recents_title") }}</div>
        <ul class="recents-list">
          <li
            v-for="r in recents"
            :key="r.path"
            :class="['recent', { missing: !r.exists }]"
            :title="r.path"
            @click="openRecent(r)"
          >
            <span class="recent-name">{{ r.name }}</span>
            <span v-if="!r.exists" class="recent-flag">{{ $t("home.recent_missing") }}</span>
          </li>
        </ul>
      </div>
    </div>
  </div>
</template>
