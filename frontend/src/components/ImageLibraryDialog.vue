<script setup lang="ts">
// Picks an existing image from the template's images folder (the reusable
// "library"), instead of uploading a new one. Lists ListImageFiles and loads a
// thumbnail per image; clicking a tile selects it. Pure reference reuse: no
// upload, no save happens here, the caller just receives the chosen filename.
import { ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import { Service as StorageSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import { backendErrMessage } from "../utils/backendError";

const props = defineProps<{
  open: boolean;
  templateFilename: string;
  current: string;
  // Optional lowercased extension allow-list (e.g. [".svg"]). When set, only
  // matching assets are shown, so a shape can browse just its SVGs.
  extensions?: string[];
}>();

const emit = defineEmits<{
  (e: "pick", name: string): void;
  (e: "close"): void;
}>();

const { t } = useI18n();

interface LibraryItem {
  name: string;
  url: string;
}

const items = ref<LibraryItem[]>([]);
const loading = ref(false);
const loadError = ref("");

async function load() {
  loading.value = true;
  loadError.value = "";
  items.value = [];
  try {
    let names = await StorageSvc.ListImageFiles(props.templateFilename);
    if (props.extensions && props.extensions.length > 0) {
      const allow = props.extensions;
      names = names.filter((n) => allow.some((ext) => n.toLowerCase().endsWith(ext)));
    }
    const loaded: LibraryItem[] = [];
    for (const name of names) {
      let url = "";
      try {
        url = (await StorageSvc.LoadImageFile(props.templateFilename, name)) ?? "";
      } catch {
        url = ""; // Still list it by name; the tile just shows a placeholder.
      }
      loaded.push({ name, url });
    }
    items.value = loaded;
  } catch (err) {
    loadError.value = backendErrMessage(err);
  } finally {
    loading.value = false;
  }
}

watch(
  () => props.open,
  (isOpen) => {
    if (isOpen && props.templateFilename) void load();
  },
  { immediate: true },
);

function choose(name: string) {
  emit("pick", name);
}

// Tile zoom: the grid's column min-width (px) driven through a CSS variable, so
// the user can scale thumbnails up to read a busy diagram or down to scan many.
const TILE_MIN = 90;
const TILE_MAX = 340;
const TILE_STEP = 50;
const tileSize = ref(120);

function zoomIn() {
  tileSize.value = Math.min(TILE_MAX, tileSize.value + TILE_STEP);
}
function zoomOut() {
  tileSize.value = Math.max(TILE_MIN, tileSize.value - TILE_STEP);
}
</script>

<template>
  <Modal
    :open="open"
    :title="t('workspace.storage.field.image_library.title')"
    width="640px"
    scroll
    maximizable
    close-on-esc
    @close="emit('close')"
  >
    <template #head>
      <div class="image-library-toolbar">
        <button
          type="button" class="btn-ghost-icon"
          :disabled="tileSize <= TILE_MIN"
          :title="t('workspace.storage.field.image_library.zoom_out')"
          @click="zoomOut"
        ><i class="fa-solid fa-magnifying-glass-minus" aria-hidden="true"></i></button>
        <button
          type="button" class="btn-ghost-icon"
          :disabled="tileSize >= TILE_MAX"
          :title="t('workspace.storage.field.image_library.zoom_in')"
          @click="zoomIn"
        ><i class="fa-solid fa-magnifying-glass-plus" aria-hidden="true"></i></button>
      </div>
    </template>
    <div v-if="loading" class="muted small">{{ t('common.loading') }}</div>
    <p v-else-if="loadError" class="form-error small">{{ loadError }}</p>
    <p v-else-if="items.length === 0" class="muted small">
      {{ t('workspace.storage.field.image_library.empty') }}
    </p>
    <div v-else class="image-library-grid" :style="{ '--lib-tile': `${tileSize}px` }">
      <button
        v-for="item in items"
        :key="item.name"
        type="button"
        class="image-library-tile"
        :class="{ active: item.name === current }"
        :title="item.name"
        @click="choose(item.name)"
      >
        <img v-if="item.url" :src="item.url" :alt="item.name" class="image-library-thumb" />
        <span v-else class="image-library-thumb image-library-thumb-empty">
          <i class="fa-regular fa-image" aria-hidden="true"></i>
        </span>
        <span class="image-library-name">{{ item.name }}</span>
      </button>
    </div>
  </Modal>
</template>
