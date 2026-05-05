<script setup lang="ts">
import { computed } from "vue";
import { FormSection, FormRow, TextField, SelectField } from "../../components/fields";
import { useConfig } from "../../composables/useConfig";

const { config, update } = useConfig();
const cfg = computed(() => config.value!);

const languages = [
  { value: "en", label: "English" },
  { value: "nl", label: "Nederlands" },
];
</script>

<template>
  <p class="section-info">Configure author details like name, email and language.</p>

  <FormSection>
    <FormRow label="Profile Name">
      <TextField
        :model-value="cfg.profile_name"
        @update:model-value="(v) => update({ profile_name: v })"
      />
    </FormRow>
    <FormRow label="Language">
      <SelectField
        :model-value="cfg.language"
        @update:model-value="(v) => update({ language: v })"
        :options="languages"
      />
    </FormRow>
    <FormRow label="Author Name">
      <TextField
        :model-value="cfg.author_name"
        @update:model-value="(v) => update({ author_name: v })"
      />
    </FormRow>
    <FormRow label="Author Email">
      <TextField
        type="email"
        :model-value="cfg.author_email"
        @update:model-value="(v) => update({ author_email: v })"
      />
    </FormRow>
  </FormSection>
</template>
