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
   *  if a command produced output - keeps shipping plugins clean. */
  enabled?: boolean;
}>();

const collapsed = ref(true);

// Two pipelines, each with its own gate:
//   - inline panel (this component) - gated by manifest.debug
//     (enabled) AND per-command hide_output / hide_log
//   - toasts - gated by per-command log_as_toast (handled by
//     the workspace, not this component)
//
// A command's block only appears here when it has at least one
// piece of visible content (output or non-empty logs respecting
// the per-command flags). When commands ran but every one had
// its data hidden, the panel surfaces a single "Not enabled"
// notice instead of a stack of orphan titles.
function hasVisibleData(
  cmd: Command,
  result: RunResultDTO | undefined,
): boolean {
  if (!result) return false;
  if (!cmd.hide_output) return true;
  const logCount = result.logLines?.length ?? 0;
  if (!cmd.hide_log && logCount > 0) return true;
  return false;
}

const cmdsWithVisibleData = computed(() =>
  props.commands.filter((c) => hasVisibleData(c, props.results[c.id])),
);

const totalShown = computed(() => cmdsWithVisibleData.value.length);

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
    v-if="enabled && totalShown > 0"
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
      <span class="plugin-result-panel-count">{{ totalShown }}</span>
    </button>

    <div v-if="!collapsed" class="plugin-result-panel-body">
      <div
        v-for="cmd in cmdsWithVisibleData"
        :key="cmd.id"
        class="plugin-result-block"
      >
        <h4 class="plugin-result-block-title">{{ cmd.label || cmd.id }}</h4>

        <template v-if="!cmd.hide_output">
          <template v-if="results[cmd.id]!.kind === 'ok'">
            <pre class="result-output">{{ prettyValue(results[cmd.id]!.value) }}</pre>
          </template>
          <template v-else>
            <h5 class="error-heading">
              {{ errorLabel(results[cmd.id]!.kind, results[cmd.id]!.message ?? '') }}
            </h5>
            <pre class="result-output error-output">{{ results[cmd.id]!.message }}</pre>
          </template>
        </template>

        <template
          v-if="!cmd.hide_log && (results[cmd.id]!.logLines?.length ?? 0) > 0"
        >
          <pre class="result-logs">{{ results[cmd.id]!.logLines!.join('\n') }}</pre>
        </template>
      </div>

    </div>
  </section>
</template>
