<script setup lang="ts">
import { useI18n } from "vue-i18n";
import { FormRow, TextField } from "./fields";
import ColorField from "./ColorField.vue";

// One row pairing the two ways a template styles its nodes in the datacore
// graph: a short label prefix and a node color. Kept side by side so the setup
// panel stays compact instead of spending a full row on each.
const prefix = defineModel<string>("prefix", { default: "" });
const color = defineModel<string>("color", { default: "" });

const { t } = useI18n();
</script>

<template>
  <FormRow
    :label="t('workspace.templates.setup.graph_node')"
    :description="t('workspace.templates.setup.graph_node_hint')"
  >
    <div class="graph-style-row">
      <TextField
        v-model="prefix"
        class="graph-style-row__prefix"
        :placeholder="t('workspace.templates.graph_prefix_placeholder')"
      />
      <ColorField
        v-model="color"
        :clear-title="t('workspace.templates.graph_color_clear')"
        :pick-title="t('workspace.templates.setup.graph_color')"
      />
    </div>
  </FormRow>
</template>
