<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import type {
  Command,
  RunResultDTO,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";

// Collapsible debug/output panel for the Run modal. Shows at the
// bottom of the dialog regardless of run mode (form vs modal),
// so the actual run UI (form fields + button, or per-cmd cards)
// stays focused on the action while the inspection surface lives
// in one shared place. Default collapsed; auto-hidden entirely
// when no command has produced visible output yet.
const { t } = useI18n();

const props = defineProps<{
  commands: Command[];
  results: Record<string, RunResultDTO>;
  /** Author opt-in. When false the panel is never rendered, even
   *  if a command produced output — keeps shipping plugins clean. */
  enabled?: boolean;
}>();

const collapsed = ref(true);

// Single gate: manifest.debug (the `enabled` prop). When on the
// panel renders every command's full output and logs regardless
// of the per-command hide_output / hide_log flags — those flags
// are end-user UX choices, but a developer with debug on wants
// to see everything. log_as_toast still surfaces lines as toasts;
// the panel just additionally shows them inline.
const cmdsWithResults = computed(() =>
  props.commands.filter((c) => props.results[c.id]),
);

function prettyValue(v: unknown): string {
  if (v === undefined || v === null) return t("workspace.plugins.no_output");
  if (typeof v === "string") return v;
  return JSON.stringify(v, null, 2);
}

function errorLabel(kind: string, message: string): string {
  if (kind === "plugin_not_found") {
    return t("workspace.plugins.error_plugin_not_found", [message]);
  }
  if (kind === "command_not_found") {
    return t("workspace.plugins.error_command_not_found", [message]);
  }
  if (kind === "server_not_running") {
    return t("workspace.plugins.error_server_not_running");
  }
  return t("workspace.plugins.error_runtime");
}
</script>

<template>
  <section
    v-if="enabled && cmdsWithResults.length > 0"
    class="plugin-result-panel"
  >
    <button
      type="button"
      class="plugin-result-panel-toggle"
      :aria-expanded="!collapsed"
      @click="collapsed = !collapsed"
    >
      <span class="plugin-result-panel-chevron" aria-hidden="true">
        {{ collapsed ? '▶' : '▼' }}
      </span>
      <span>{{ t('workspace.plugins.debug.title') }}</span>
      <span class="plugin-result-panel-count">{{ cmdsWithResults.length }}</span>
    </button>

    <div v-if="!collapsed" class="plugin-result-panel-body">
      <div
        v-for="cmd in cmdsWithResults"
        :key="cmd.id"
        class="plugin-result-block"
      >
        <h4 class="plugin-result-block-title">{{ cmd.label || cmd.id }}</h4>

        <template v-if="results[cmd.id]!.kind === 'ok'">
          <pre class="result-output">{{ prettyValue(results[cmd.id]!.value) }}</pre>
        </template>
        <template v-else>
          <h5 class="error-heading">
            {{ errorLabel(results[cmd.id]!.kind, results[cmd.id]!.message ?? '') }}
          </h5>
          <pre class="result-output error-output">{{ results[cmd.id]!.message }}</pre>
        </template>

        <template v-if="(results[cmd.id]!.logLines?.length ?? 0) > 0">
          <pre class="result-logs">{{ results[cmd.id]!.logLines!.join('\n') }}</pre>
        </template>
      </div>
    </div>
  </section>
</template>
