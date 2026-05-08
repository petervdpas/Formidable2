<script setup lang="ts">
import { useI18n } from "vue-i18n";
import SwitchField from "./fields/SwitchField.vue";
import type { Command } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";

// One row for editing a single Command in the manifest. Top line:
// labelled Id / Label / Lua-function inputs + a delete button.
// Bottom line: Result + Log toggle-switches that map to the
// command's hide_output / hide_log flags (inverted — the user sees
// "Show, ON by default", JSON stores hide_*).
//
// We mutate the passed-in `command` object directly, which works
// because Vue 3 keeps nested-prop reactivity intact (same pattern
// the templates workspace uses for its field rows). The only
// outbound event is `delete`.
const props = defineProps<{ command: Command }>();
defineEmits<{ (e: "delete"): void }>();

const { t } = useI18n();

// Show Log and Log as Toast are mutually exclusive: turning one
// on snaps the other off. The two flags carry different meanings
// (inline panel vs. live notifications), so showing both at once
// is just noise — pick one.
function setShowLog(v: boolean) {
  props.command.hide_log = !v;
  if (v) props.command.log_as_toast = false;
}

function setLogAsToast(v: boolean) {
  props.command.log_as_toast = v;
  if (v) props.command.hide_log = true;
}
</script>

<template>
  <li class="cmd-row">
    <div class="cmd-row-grid">
      <label class="cmd-field">
        <span class="cmd-field-label">{{ t('workspace.plugins.commands.id') }}</span>
        <input class="field-input cmd-input" v-model="command.id" />
      </label>
      <label class="cmd-field">
        <span class="cmd-field-label">{{ t('workspace.plugins.commands.label') }}</span>
        <input class="field-input cmd-input" v-model="command.label" />
      </label>
      <label class="cmd-field">
        <span class="cmd-field-label">{{ t('workspace.plugins.commands.fn') }}</span>
        <input
          class="field-input cmd-input"
          v-model="command.fn"
          :placeholder="command.id || t('workspace.plugins.commands.fn_placeholder')"
        />
      </label>
      <button
        type="button"
        class="cmd-delete-btn"
        :title="t('workspace.plugins.commands.delete')"
        :aria-label="t('workspace.plugins.commands.delete')"
        @click="$emit('delete')"
      >×</button>
    </div>

    <div class="cmd-toggles-row">
      <label
        class="cmd-toggle"
        :title="t('workspace.plugins.commands.form_button_title')"
      >
        <span>{{ t('workspace.plugins.commands.form_button') }}</span>
        <SwitchField v-model="command.form_button" />
      </label>
      <label
        class="cmd-toggle"
        :title="t('workspace.plugins.commands.show_output_title')"
      >
        <span>{{ t('workspace.plugins.commands.show_output') }}</span>
        <SwitchField
          :model-value="!command.hide_output"
          @update:model-value="(v) => (command.hide_output = !v)"
        />
      </label>
      <label
        class="cmd-toggle"
        :title="t('workspace.plugins.commands.show_log_title')"
      >
        <span>{{ t('workspace.plugins.commands.show_log') }}</span>
        <SwitchField
          :model-value="!command.hide_log"
          @update:model-value="setShowLog"
        />
      </label>
      <label
        class="cmd-toggle"
        :title="t('workspace.plugins.commands.log_as_toast_title')"
      >
        <span>{{ t('workspace.plugins.commands.log_as_toast') }}</span>
        <SwitchField
          :model-value="command.log_as_toast"
          @update:model-value="setLogAsToast"
        />
      </label>
    </div>
  </li>
</template>
