<script setup lang="ts">
import { useI18n } from "vue-i18n";
import type { Command } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";

// One compact row for editing a single Command in the manifest. The
// parent passes the Command instance directly — we mutate its
// fields in place, which works because Vue 3 makes nested props
// reactive (same pattern as the inline editor we replaced). The
// only outbound event is `delete`; everything else flows via the
// shared object.
defineProps<{ command: Command }>();
defineEmits<{ (e: "delete"): void }>();

const { t } = useI18n();
</script>

<template>
  <li class="cmd-row cmd-row--compact">
    <input
      class="field-input cmd-input cmd-input--id"
      v-model="command.id"
      :placeholder="t('workspace.plugins.commands.id')"
      :title="t('workspace.plugins.commands.id')"
    />
    <input
      class="field-input cmd-input cmd-input--label"
      v-model="command.label"
      :placeholder="t('workspace.plugins.commands.label')"
      :title="t('workspace.plugins.commands.label')"
    />
    <input
      class="field-input cmd-input cmd-input--fn"
      v-model="command.fn"
      :placeholder="command.id || t('workspace.plugins.commands.fn')"
      :title="t('workspace.plugins.commands.fn')"
    />
    <label
      class="cmd-toggle"
      :title="t('workspace.plugins.commands.show_output_title')"
    >
      <input
        type="checkbox"
        :checked="!command.hide_output"
        @change="(e) => (command.hide_output = !(e.target as HTMLInputElement).checked)"
      />
      <span>{{ t('workspace.plugins.commands.show_output_short') }}</span>
    </label>
    <label
      class="cmd-toggle"
      :title="t('workspace.plugins.commands.show_log_title')"
    >
      <input
        type="checkbox"
        :checked="!command.hide_log"
        @change="(e) => (command.hide_log = !(e.target as HTMLInputElement).checked)"
      />
      <span>{{ t('workspace.plugins.commands.show_log_short') }}</span>
    </label>
    <button
      type="button"
      class="cmd-delete-btn"
      :title="t('workspace.plugins.commands.delete')"
      :aria-label="t('workspace.plugins.commands.delete')"
      @click="$emit('delete')"
    >×</button>
  </li>
</template>
