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
    const names = await StorageSvc.ListImageFiles(props.templateFilename);
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
    <div v-if="loading" class="muted small">{{ t('common.loading') }}</div>
    <p v-else-if="loadError" class="form-error small">{{ loadError }}</p>
    <p v-else-if="items.length === 0" class="muted small">
      {{ t('workspace.storage.field.image_library.empty') }}
    </p>
    <div v-else class="image-library-grid">
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
