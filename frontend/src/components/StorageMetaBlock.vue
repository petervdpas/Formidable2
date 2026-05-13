<script setup lang="ts">
/*
 * StorageMetaBlock — the per-record meta panel rendered above a
 * storage entry's form fields. Shows filename, flag picker, GUID,
 * tags, and the Created / Updated audit blocks (timestamp + author
 * name + email). Hidden via Mod+M on the Storage workspace (which
 * flips config.show_meta_section).
 *
 * Pure presentation: parent owns the draft + flag definitions and
 * handles the flag mutation. We emit `flagStateChange` instead of
 * mutating the meta object directly so the workspace's dirty-state
 * tracking stays the single source of truth.
 */
import { useI18n } from "vue-i18n";
import { FormSection } from "./fields";
import FlagPicker from "./FlagPicker.vue";
import type { FormMeta } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import type { FlagDefinition } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

defineProps<{
  datafile?: string;
  meta?: FormMeta | null;
  flagDefinitions: FlagDefinition[];
}>();

defineEmits<{
  (e: "flagStateChange", state: string): void;
}>();

const { t } = useI18n();
</script>

<template>
  <FormSection class="storage-meta-section">
    <div
      class="meta-flag-corner"
      v-if="flagDefinitions.length > 0 || meta?.flagged"
    >
      <FlagPicker
        :definitions="flagDefinitions"
        :model-value="meta?.flag_state ?? ''"
        :legacy-flagged="!!meta?.flagged"
        size="md"
        placement="below-left"
        @update:model-value="(s: string) => $emit('flagStateChange', s)"
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
            — {{ meta.created.name }}<template v-if="meta.created.email"> ({{ meta.created.email }})</template>
          </template>
        </span>
      </div>
      <div class="meta-row" v-if="meta?.updated?.at">
        <span class="meta-key">{{ t('workspace.storage.meta.updated') }}</span>
        <span class="meta-value small">
          <span class="mono">{{ meta.updated.at }}</span>
          <template v-if="meta.updated.name">
            — {{ meta.updated.name }}<template v-if="meta.updated.email"> ({{ meta.updated.email }})</template>
          </template>
        </span>
      </div>
    </div>
  </FormSection>
</template>
