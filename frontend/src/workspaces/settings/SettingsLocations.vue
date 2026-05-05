<script setup lang="ts">
import { computed } from "vue";
import { FormSection, FormRow, TextField, SelectField } from "../../components/fields";
import { useConfig } from "../../composables/useConfig";

const { config, update } = useConfig();
const cfg = computed(() => config.value!);

const backends = [
  { value: "none",  label: "None" },
  { value: "git",   label: "Git" },
  { value: "gigot", label: "GiGot" },
];

const isGigot = computed(() => cfg.value.remote_backend === "gigot");
</script>

<template>
  <p class="section-info">Configure the context folder and the optional remote backend (Git or GiGot).</p>

  <FormSection>
    <FormRow label="Context Directory" description="Root folder for templates and form storage.">
      <TextField
        :model-value="cfg.context_folder"
        @update:model-value="(v) => update({ context_folder: v })"
        placeholder="/path/to/context"
      />
    </FormRow>

    <FormRow label="Remote Backend">
      <SelectField
        :model-value="cfg.remote_backend"
        @update:model-value="(v) => update({ remote_backend: v })"
        :options="backends"
      />
    </FormRow>

    <FormRow v-if="isGigot" label="GiGot Base URL">
      <TextField
        :model-value="cfg.gigot_base_url"
        @update:model-value="(v) => update({ gigot_base_url: v })"
        placeholder="https://gigot.example.com"
      />
    </FormRow>
    <FormRow v-if="isGigot" label="GiGot Repository">
      <TextField
        :model-value="cfg.gigot_repo_name"
        @update:model-value="(v) => update({ gigot_repo_name: v })"
      />
    </FormRow>
    <FormRow v-if="isGigot" label="GiGot Subscription Token">
      <TextField
        type="password"
        :model-value="cfg.gigot_token"
        @update:model-value="(v) => update({ gigot_token: v })"
      />
    </FormRow>
  </FormSection>
</template>
