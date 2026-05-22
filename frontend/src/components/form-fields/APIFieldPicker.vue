<script setup lang="ts">
// Modal record-picker for an api field. Lists every collection-enabled
// record from the source template (via the existing /api/collections/<tpl>
// HTTP route - already plumbed through Wails AssetMiddleware) and lets
// the user pick one by guid. The host form's FormFieldAPI takes that
// guid and stamps the projected row through dataprovider.FetchAPIFieldRow.
//
// v1: fetch up to 200 items (limit query). q-filter / pagination /
// tag-filter come later - the api Handler already supports them.

import { ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "../Modal.vue";

const props = defineProps<{
  open: boolean;
  /** Source template filename ("people.yaml"). */
  sourceTemplate: string;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "pick", guid: string): void;
}>();

const { t } = useI18n();

type CollectionItem = {
  template: string;
  id: string;
  filename: string;
  title: string;
  tags?: string[];
  hrefSelf: string;
  hrefHtml: string;
};

const items = ref<CollectionItem[]>([]);
const loading = ref(false);
const error = ref("");
const filter = ref("");

async function load() {
  if (!props.sourceTemplate) {
    items.value = [];
    return;
  }
  loading.value = true;
  error.value = "";
  try {
    // /api/* is delegated by AssetMiddleware to the api Handler in-process.
    const stem = props.sourceTemplate.replace(/\.yaml$/, "");
    const res = await fetch(`/api/collections/${encodeURIComponent(stem)}?limit=200`);
    if (!res.ok) {
      throw new Error(`HTTP ${res.status}`);
    }
    const body = (await res.json()) as { items?: CollectionItem[] };
    items.value = body.items ?? [];
  } catch (e) {
    error.value = String((e as Error)?.message ?? e);
    items.value = [];
  } finally {
    loading.value = false;
  }
}

// Reload whenever the picker opens against a (possibly different)
// source template.
watch(
  () => [props.open, props.sourceTemplate] as const,
  ([open]) => {
    if (open) {
      filter.value = "";
      void load();
    }
  },
);

function pick(guid: string) {
  emit("pick", guid);
  emit("close");
}

const filteredItems = () => {
  const q = filter.value.trim().toLowerCase();
  if (!q) return items.value;
  return items.value.filter((it) => {
    const hay = (it.title + " " + (it.tags ?? []).join(" ")).toLowerCase();
    return hay.includes(q);
  });
};
</script>

<template>
  <Modal
    :open="open"
    width="560px"
    :title="t('workspace.storage.api_picker.title')"
    @close="emit('close')"
  >
    <div class="api-picker">
      <input
        type="search"
        v-model="filter"
        :placeholder="t('workspace.storage.api_picker.filter_placeholder')"
        class="api-picker-search"
      />
      <p v-if="loading" class="muted small">{{ t("shell.common.loading") }}</p>
      <p v-if="error" class="error small">{{ error }}</p>
      <p
        v-if="!loading && !error && items.length === 0"
        class="muted small"
      >
        {{ t("workspace.storage.api_picker.empty") }}
      </p>
      <ul v-if="items.length > 0" class="api-picker-list">
        <li v-for="it in filteredItems()" :key="it.id">
          <button
            type="button"
            class="api-picker-row"
            @click="pick(it.id)"
          >
            <span class="api-picker-title">{{ it.title || it.filename }}</span>
            <span v-if="it.tags?.length" class="api-picker-tags">
              <span v-for="tag in it.tags" :key="tag" class="tag-chip">{{ tag }}</span>
            </span>
            <span class="api-picker-guid muted">{{ it.id }}</span>
          </button>
        </li>
      </ul>
    </div>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('close')">
        {{ t("common.cancel") }}
      </button>
    </template>
  </Modal>
</template>

<style scoped>
.api-picker {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 60vh;
  overflow: hidden;
}
.api-picker-search {
  width: 100%;
  padding: 6px 10px;
  background: var(--input-bg, transparent);
  color: var(--input-fg, currentColor);
  border: 1px solid color-mix(in oklab, currentColor 25%, transparent);
  border-radius: 6px;
}
.api-picker-list {
  list-style: none;
  margin: 0;
  padding: 0;
  overflow-y: auto;
  max-height: 50vh;
}
.api-picker-row {
  width: 100%;
  display: grid;
  grid-template-columns: 1fr auto;
  grid-template-rows: auto auto;
  gap: 2px 12px;
  align-items: center;
  padding: 8px 10px;
  background: transparent;
  color: inherit;
  border: 0;
  border-bottom: 1px solid color-mix(in oklab, currentColor 12%, transparent);
  text-align: left;
  cursor: pointer;
}
.api-picker-row:hover {
  background: color-mix(in oklab, currentColor 8%, transparent);
}
.api-picker-title {
  font-weight: 500;
  grid-column: 1;
  grid-row: 1;
}
.api-picker-tags {
  grid-column: 2;
  grid-row: 1;
  display: inline-flex;
  gap: 4px;
}
.tag-chip {
  display: inline-block;
  padding: 1px 6px;
  border-radius: 999px;
  background: color-mix(in oklab, currentColor 14%, transparent);
  font-size: 0.75rem;
}
.api-picker-guid {
  grid-column: 1 / -1;
  grid-row: 2;
  font-size: 0.75rem;
  font-family: ui-monospace, SFMono-Regular, monospace;
}
.error {
  color: var(--color-danger, #c0392b);
}
</style>
