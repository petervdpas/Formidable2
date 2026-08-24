<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { FormSection, FormRow, FolderPathField, SelectField } from "../../components/fields";
import { useConfig } from "../../composables/useConfig";
import { LOCATION_LABEL_KEYS } from "../../composables/useSavedQueries";
import { Service as QuerySvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/query";

const { t } = useI18n();
const { config, update } = useConfig();
const cfg = computed(() => config.value!);

const backends = computed(() => [
  { value: "none",  label: t("backend.none") },
  { value: "git",   label: t("backend.git") },
  { value: "gigot", label: t("backend.gigot") },
]);

// The saved-query locations are a closed backend-owned set; fetch it rather
// than restating it here. The label map is explicit (never an interpolated
// lookup key), so a location without one falls back to its raw value.
const queryLocations = ref<string[]>([]);
onMounted(async () => {
  queryLocations.value = (await QuerySvc.SavedQueryLocations()) ?? [];
});
const queryLocationOptions = computed(() =>
  queryLocations.value.map((v) => ({
    value: v,
    label: LOCATION_LABEL_KEYS[v] ? t(LOCATION_LABEL_KEYS[v]) : v,
  })),
);
</script>

<template>
  <p class="section-info">{{ t('settings.locations.info') }}</p>

  <FormSection>
    <FormRow
      :label="t('settings.field.context_directory')"
      :description="t('settings.desc.context_directory')"
    >
      <FolderPathField
        :model-value="cfg.context_folder"
        @update:model-value="(v) => update({ context_folder: v })"
        placeholder="/path/to/context"
      />
    </FormRow>

    <FormRow
      :label="t('settings.field.remote_backend')"
      :description="t('settings.desc.remote_backend')"
    >
      <SelectField
        :model-value="cfg.remote_backend"
        @update:model-value="(v) => update({ remote_backend: v })"
        :options="backends"
      />
    </FormRow>

    <FormRow
      :label="t('settings.field.saved_query_location')"
      :description="t('settings.desc.saved_query_location')"
    >
      <SelectField
        :model-value="cfg.saved_query_location"
        @update:model-value="(v) => update({ saved_query_location: v })"
        :options="queryLocationOptions"
      />
    </FormRow>
  </FormSection>
</template>
