<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import { useRestartGate } from "../composables/useRestartGate";
import {
  Service as PluginSvc,
  RunResultDTO,
  type ListResult,
  type Command,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";

const { t } = useI18n();
const { bootConfig } = useRestartGate();

const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

// Plugins discovered by the backend's Manager.Refresh. Sorted by
// id (the Service guarantees stable order).
const plugins = ref<ListResult[]>([]);
const selectedID = ref<string>("");

const selected = computed(
  () => plugins.value.find((p) => p.id === selectedID.value),
);

// Latest run result keyed by `${pluginID}:${commandID}` so the
// output panel survives switching between commands of the same
// plugin without re-running. A user re-running clears its own slot.
const results = ref<Record<string, RunResultDTO>>({});
const runningKey = ref<string>("");

async function refresh() {
  try {
    plugins.value = await PluginSvc.Refresh();
    if (!selected.value && plugins.value.length > 0) {
      selectedID.value = plugins.value[0].id;
    }
  } catch (err) {
    // Surface the discovery error in the same panel so users see
    // why the list is empty without digging into the dev console.
    plugins.value = [];
    results.value["__refresh"] = new RunResultDTO({
      kind: "runtime_error",
      message: String(err),
    });
  }
}

async function run(pluginID: string, cmd: Command) {
  const key = `${pluginID}:${cmd.id}`;
  runningKey.value = key;
  try {
    results.value[key] = await PluginSvc.Run(pluginID, cmd.id, {});
  } catch (err) {
    results.value[key] = new RunResultDTO({
      kind: "runtime_error",
      message: String(err),
    });
  } finally {
    runningKey.value = "";
  }
}

function resultFor(pluginID: string, cmdID: string): RunResultDTO | undefined {
  return results.value[`${pluginID}:${cmdID}`];
}

function isRunning(pluginID: string, cmdID: string): boolean {
  return runningKey.value === `${pluginID}:${cmdID}`;
}

function prettyValue(v: unknown): string {
  if (v === undefined || v === null) {
    return t("workspace.plugins.no_output");
  }
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
  return t("workspace.plugins.error_runtime");
}

onMounted(() => {
  void refresh();
});
</script>

<template>
  <Teleport defer to="#topbar-content">
    <span class="topbar-spacer"></span>
    <div class="topbar-actions">
      <button class="tool-btn" @click="refresh">
        {{ t('common.refresh') }}
      </button>
    </div>
  </Teleport>

  <SplitPane :initial="sidebarWidth">
    <template #sidebar>
      <h2 class="sidebar-title">{{ t('workspace.plugins.sidebar_title') }}</h2>
      <ul v-if="plugins.length > 0" class="sidebar-list">
        <li
          v-for="p in plugins"
          :key="p.id"
          :class="['sidebar-row', { active: p.id === selectedID }]"
          @click="selectedID = p.id"
        >
          <div class="plugin-name">{{ p.manifest.name }}</div>
          <div class="plugin-meta muted small">
            {{ t('workspace.plugins.version_label', [p.manifest.version]) }}
          </div>
        </li>
      </ul>
      <p v-else class="muted small" v-html="t('workspace.plugins.empty_side', ['plugins/'])"></p>
    </template>

    <template #main>
      <p v-if="!selected" class="workspace-empty">
        {{ t('workspace.plugins.empty_main') }}
      </p>
      <div v-else class="plugin-detail">
        <header class="plugin-header">
          <h1 class="workspace-heading">{{ selected.manifest.name }}</h1>
          <div class="muted small">
            {{ t('workspace.plugins.version_label', [selected.manifest.version]) }}
            <span v-if="selected.manifest.author"> — {{ selected.manifest.author }}</span>
          </div>
          <p v-if="selected.manifest.description" class="plugin-description">
            {{ selected.manifest.description }}
          </p>
        </header>

        <section
          v-for="cmd in selected.manifest.commands"
          :key="cmd.id"
          class="command-card"
        >
          <div class="command-header">
            <h3>{{ cmd.label }}</h3>
            <button
              class="tool-btn primary"
              :disabled="isRunning(selected.id, cmd.id)"
              @click="run(selected.id, cmd)"
            >
              <span v-if="isRunning(selected.id, cmd.id)">
                {{ t('workspace.plugins.running') }}
              </span>
              <span v-else>{{ t('workspace.plugins.run') }}</span>
            </button>
          </div>

          <div v-if="resultFor(selected.id, cmd.id)" class="command-result">
            <template v-if="resultFor(selected.id, cmd.id)!.kind === 'ok'">
              <h4>{{ t('workspace.plugins.output_title') }}</h4>
              <pre class="result-output">{{ prettyValue(resultFor(selected.id, cmd.id)!.value) }}</pre>
            </template>
            <template v-else>
              <h4 class="error-heading">
                {{
                  errorLabel(
                    resultFor(selected.id, cmd.id)!.kind,
                    resultFor(selected.id, cmd.id)!.message ?? '',
                  )
                }}
              </h4>
              <pre class="result-output error-output">{{ resultFor(selected.id, cmd.id)!.message }}</pre>
            </template>

            <template v-if="(resultFor(selected.id, cmd.id)!.logLines?.length ?? 0) > 0">
              <h4>{{ t('workspace.plugins.logs_title') }}</h4>
              <pre class="result-logs">{{ resultFor(selected.id, cmd.id)!.logLines!.join('\n') }}</pre>
            </template>
          </div>
        </section>
      </div>
    </template>
  </SplitPane>
</template>

