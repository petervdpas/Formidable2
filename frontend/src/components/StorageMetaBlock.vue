<script setup lang="ts">
/*
 * StorageMetaBlock - the per-record meta panel rendered above a
 * storage entry's form fields. Shows filename, one FacetPicker per
 * template facet, GUID, tags, and the Created / Updated audit blocks.
 * Hidden via Mod+M on the Storage workspace.
 *
 * Pure presentation: parent owns the draft + template facets and
 * handles the facet-state mutation. We emit `facetStateChange` instead
 * of mutating the meta object directly so the workspace's dirty-state
 * tracking stays the single source of truth.
 */
import { useI18n } from "vue-i18n";
import { FormSection } from "./fields";
import FacetPicker from "./FacetPicker.vue";
import type { FormMeta } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import { FacetState } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import type { Facet } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  datafile?: string;
  meta?: FormMeta | null;
  facets: Facet[];
}>();

const emit = defineEmits<{
  (e: "facetStateChange", key: string, state: FacetState): void;
}>();

const { t } = useI18n();

function stateFor(key: string): FacetState {
  const entry = props.meta?.facets?.[key];
  if (!entry) return new FacetState({ set: false, selected: "" });
  return new FacetState({ set: entry.set, selected: entry.selected ?? "" });
}

function onUpdate(key: string, state: FacetState) {
  emit("facetStateChange", key, state);
}
</script>

<template>
  <FormSection class="storage-meta-section">
    <div
      v-if="facets.length > 0"
      class="meta-facet-corner"
    >
      <FacetPicker
        v-for="f in facets"
        :key="f.key"
        :facet="f"
        :model-value="stateFor(f.key)"
        size="md"
        placement="below-left"
        @update:model-value="(s: FacetState) => onUpdate(f.key, s)"
      />
    </div>
    <div class="meta-grid">
      <div class="meta-row" v-if="datafile">
        <span class="meta-key">{{ t('workspace.storage.meta.filename') }}</span>
        <span class="meta-value mono">{{ datafile }}</span>
      </div>
      <div class="meta-row" v-if="meta?.id">
        <span class="meta-key">{{ t('workspace.storage.meta.id') }}</span>
        <span class="meta-value mono">{{ meta.id }}</span>
      </div>
      <div class="meta-row" v-if="meta?.tags?.length">
        <span class="meta-key">{{ t('workspace.storage.meta.tags') }}</span>
        <span class="meta-value">{{ meta.tags.join(', ') }}</span>
      </div>
      <div class="meta-row" v-if="meta?.created?.at">
        <span class="meta-key">{{ t('workspace.storage.meta.created') }}</span>
        <span class="meta-value small">
          <span class="mono">{{ meta.created.at }}</span>
          <template v-if="meta.created.name">
            - {{ meta.created.name }}<template v-if="meta.created.email"> ({{ meta.created.email }})</template>
          </template>
        </span>
      </div>
      <div class="meta-row" v-if="meta?.updated?.at">
        <span class="meta-key">{{ t('workspace.storage.meta.updated') }}</span>
        <span class="meta-value small">
          <span class="mono">{{ meta.updated.at }}</span>
          <template v-if="meta.updated.name">
            - {{ meta.updated.name }}<template v-if="meta.updated.email"> ({{ meta.updated.email }})</template>
          </template>
        </span>
      </div>
    </div>
  </FormSection>
</template>
