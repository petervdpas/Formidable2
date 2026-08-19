<script setup lang="ts">
// Modal record-picker for an api-client field. Lists candidates straight from
// the remote service through the client's declared list binding, so the search
// box is the remote's own search when the resource declares one.

import { ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "../Modal.vue";
import { useAPIClients } from "../../composables/useAPIClients";
import type { Item } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/connection";

const props = defineProps<{
  open: boolean;
  clientId: string;
  resource: string;
  /** Projected field keys to request, so the rows preview what will be stored. */
  select: string[];
  /** Resolved runtime parameters for the list call. */
  params: Record<string, string>;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "pick", id: string): void;
}>();

const { t } = useI18n();
const clients = useAPIClients();

const items = ref<Item[]>([]);
const loading = ref(false);
const error = ref("");
const search = ref("");

const LIMIT = 100;

async function load() {
  if (!props.clientId || !props.resource) {
    items.value = [];
    return;
  }
  loading.value = true;
  error.value = "";
  const res = await clients.listItems({
    connection: props.clientId,
    resource: props.resource,
    search: search.value,
    limit: LIMIT,
    offset: 0,
    cursor: "",
    select: props.select,
    params: props.params,
  });
  loading.value = false;
  if (!res.ok) {
    error.value = res.message;
    items.value = [];
    return;
  }
  items.value = res.page?.items ?? [];
}

watch(
  () => props.open,
  (open) => {
    if (!open) return;
    search.value = "";
    void load();
  },
  { immediate: true },
);

function preview(item: Item): string {
  const fields = item.fields ?? {};
  return props.select
    .map((k) => fields[k])
    .filter((v) => v != null && v !== "")
    .join(" · ");
}
</script>

<template>
  <Modal
    :open="open"
    :title="t('workspace.storage.api_client_field.picker_title')"
    width="620px"
    @close="emit('close')"
  >
    <div class="api-client-picker">
      <form class="api-client-picker-search-row" @submit.prevent="load">
        <input
          v-model="search"
          class="api-client-picker-search"
          type="search"
          :placeholder="t('workspace.storage.api_client_field.search')"
        />
        <button type="submit" class="tool-btn small" :disabled="loading">
          {{ t('workspace.storage.api_client_field.search_go') }}
        </button>
      </form>

      <p v-if="loading" class="muted small">{{ t('shell.common.loading') }}</p>
      <p v-else-if="error" class="error small">{{ error }}</p>
      <p v-else-if="!items.length" class="muted small">
        {{ t('workspace.storage.api_client_field.no_results') }}
      </p>

      <ul v-else class="api-client-picker-list">
        <li v-for="item in items" :key="item.id">
          <button type="button" class="api-client-picker-row" @click="emit('pick', item.id)">
            <span class="api-client-picker-title">{{ item.label || item.id }}</span>
            <span v-if="preview(item)" class="api-client-picker-preview">{{ preview(item) }}</span>
            <span class="api-client-picker-id">{{ item.id }}</span>
          </button>
        </li>
      </ul>
    </div>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('close')">
        {{ t('common.cancel') }}
      </button>
    </template>
  </Modal>
</template>
