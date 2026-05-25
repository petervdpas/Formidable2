<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import FormFieldRow from "./form-fields/FormFieldRow.vue";
import PluginResultPanel from "./PluginResultPanel.vue";
import {
  Service as PluginSvc,
  Command,
  RunResultDTO,
  type ListResult,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";
import { Service as RenderSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/render";
import type { Field } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { getFieldTypeDef } from "../types/field-types";
import {
  useGlobalPluginRun,
  closeGlobalPluginRun,
  setGlobalPluginRunning,
  cancelGlobalPluginRun,
} from "../composables/useGlobalPluginRun";
import { useToast } from "../composables/useToast";
import ProgressBarWidget from "./widgets/ProgressBarWidget.vue";
import StatusMessageWidget from "./widgets/StatusMessageWidget.vue";
import type { Widget } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/formwidget";
import { Kind as WidgetKind } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/formwidget";
import { isWidget } from "../composables/usePluginEditor";
import { pluginName, pluginDescription, commandLabel } from "../utils/pluginI18n";
import StatChartDialog from "./stat/StatChartDialog.vue";
import { extractCharts, type ChartEnvelope } from "./stat/types";
import ChartWidget from "./widgets/ChartWidget.vue";

// PluginRunDialog mirrors PluginsWorkspace's inline Run modal but is
// mounted once at the App level and driven entirely by the manifest
// of the plugin in `openRequest`. The two modes the manifest declares
// are honored verbatim - there is no filtering on top:
//   - run_mode === "form" → render the plugin's form (with the
//     description above it) and each form_button command as an action
//     button. ctx = { ...extraCtx, ...formValues }.
//   - else                → render one card per non-form_button
//     command, ctx = { ...extraCtx }.
//
// Workspace topbar menu items pass extraCtx = { workspace: "<id>" } so
// the Lua side can branch on its invocation context.

const { t } = useI18n();
const toast = useToast();
const { openRequest, running, stopping } = useGlobalPluginRun();

const plugin = computed<ListResult | null>(() => openRequest.value?.plugin ?? null);
const extraCtx = computed<Record<string, unknown>>(() => openRequest.value?.extraCtx ?? {});

// formEntries is the heterogeneous parsed form.json - each entry is
// either a Field (input row) or a Widget (live display slot). Order
// in the array IS the render order; no separate sort key.
const formEntries = ref<Array<Field | Widget>>([]);
const runValues = ref<Record<string, unknown>>({});
const runResults = ref<Record<string, RunResultDTO>>({});
const runningCmd = ref<string>("");
const descriptionHTML = ref<string>("");

// Chart output: when a command returns a chart envelope (see
// extractCharts), the host opens this glance-and-close dialog over the
// run modal instead of leaving the data buried in the debug panel.
const chartList = ref<ChartEnvelope[]>([]);
const chartTitle = ref<string>("");
const chartOpen = ref(false);

function closeChart() {
  chartOpen.value = false;
  chartList.value = [];
  chartTitle.value = "";
}

async function stopRun() {
  await cancelGlobalPluginRun();
}

const runMode = computed(
  () => (plugin.value?.manifest.run_mode || "modal") as "modal" | "form",
);

const visibleCommands = computed(() => {
  const all = plugin.value?.manifest.commands ?? [];
  if (runMode.value === "form") {
    return all.filter((c) => c.form_button);
  }
  return all.filter((c) => !c.form_button);
});

function initialRunValues(entries: Array<Field | Widget>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const e of entries) {
    if (isWidget(e)) continue;
    const f = e;
    if (!f.key) continue;
    if (f.default !== undefined && f.default !== null) {
      out[f.key] = f.default;
      continue;
    }
    const def = getFieldTypeDef(f.type)?.defaultValue?.();
    out[f.key] = def !== undefined ? def : "";
  }
  return out;
}

// Open → reset state, load form schema + KV-saved values from disk.
// Cleared on close (openRequest goes null) so the next open doesn't
// flash stale data.
watch(
  plugin,
  async (p) => {
    runResults.value = {};
    runningCmd.value = "";
    if (!p) {
      formEntries.value = [];
      runValues.value = {};
      descriptionHTML.value = "";
      return;
    }
    let entries: Array<Field | Widget> = [];
    try {
      const raw = await PluginSvc.GetForm(p.id);
      const parsed = JSON.parse(raw || "[]");
      if (Array.isArray(parsed)) {
        entries = parsed;
      }
    } catch {
      entries = [];
    }
    formEntries.value = entries;
    let values = initialRunValues(entries);
    const keys = entries
      .filter((e): e is Field => !isWidget(e))
      .map((f) => f.key)
      .filter((k): k is string => !!k);
    if (keys.length > 0) {
      try {
        const saved = await PluginSvc.LoadFormValues(p.id, keys);
        for (const k of keys) {
          if (saved && saved[k] !== undefined) values[k] = saved[k];
        }
      } catch {
        /* fall back to defaults */
      }
    }
    runValues.value = values;

    const md = pluginDescription(p).trim();
    if (md) {
      try {
        descriptionHTML.value = await RenderSvc.RenderHTML(md);
      } catch {
        descriptionHTML.value = md;
      }
    } else {
      descriptionHTML.value = "";
    }
  },
  { immediate: true },
);

async function runCommand(cmd: Command) {
  const p = plugin.value;
  if (!p) return;
  runningCmd.value = cmd.id;
  setGlobalPluginRunning(true);
  try {
    const isForm = runMode.value === "form";
    const ctx: Record<string, unknown> = isForm
      ? { ...extraCtx.value, ...runValues.value }
      : { ...extraCtx.value };
    if (isForm) {
      try {
        await PluginSvc.SaveFormValues(p.id, { ...runValues.value });
      } catch {
        /* best-effort; don't block the run */
      }
    }
    const res = await PluginSvc.Run(p.id, cmd.id, ctx);
    runResults.value[cmd.id] = res;
    if (res.kind === "ok") {
      const charts = extractCharts(res.value);
      if (charts.length > 0) {
        chartList.value = charts;
        chartTitle.value = plugin.value
          ? commandLabel(plugin.value.id, cmd)
          : cmd.label || cmd.id;
        chartOpen.value = true;
      }
    }
    if (res.kind === "busy") {
      toast.warn(res.message || "plugin: another command is currently running");
    } else if (res.kind === "cancelled") {
      toast.info("workspace.plugins.cancelled");
    }
    for (const ev of res.toasts ?? []) {
      const fn = toast[ev.level as "info" | "success" | "warn" | "error"];
      if (fn) fn(ev.message);
    }
    if (cmd.log_as_toast) {
      for (const line of res.logLines ?? []) {
        const m = /^\[(\w+)\]\s*(.*)$/.exec(line);
        const level = (m?.[1] ?? "info").toLowerCase();
        const msg = m?.[2] ?? line;
        const variant: "info" | "success" | "warn" | "error" =
          level === "warn"
            ? "warn"
            : level === "error"
              ? "error"
              : "info";
        toast[variant](msg);
      }
    }
  } catch (err) {
    runResults.value[cmd.id] = new RunResultDTO({
      kind: "runtime_error",
      message: String(err),
    });
  } finally {
    runningCmd.value = "";
    setGlobalPluginRunning(false);
  }
}

function close() {
  closeChart();
  closeGlobalPluginRun();
}

</script>

<template>
  <Modal
    :open="!!plugin"
    :title="
      plugin
        ? runMode === 'form'
          ? pluginName(plugin)
          : t('workspace.plugins.run_title', [pluginName(plugin)])
        : ''
    "
    width="640px"
    :maximizable="!!plugin?.manifest.maximizable && runMode === 'form'"
    @close="close"
  >
    <div v-if="plugin" class="run-modal">
      <section v-if="runMode === 'form'" class="run-form">
        <div
          v-if="descriptionHTML"
          class="section-info"
          v-html="descriptionHTML"
        ></div>
        <template v-for="(entry, i) in formEntries" :key="i">
          <ProgressBarWidget
            v-if="isWidget(entry) && entry.kind === WidgetKind.KindProgressBar"
            :widget="entry"
          />
          <StatusMessageWidget
            v-else-if="isWidget(entry) && entry.kind === WidgetKind.KindStatusMessage"
            :widget="entry"
          />
          <ChartWidget
            v-else-if="isWidget(entry) && entry.kind === WidgetKind.KindChart"
            :widget="entry"
          />
          <FormFieldRow
            v-else-if="!isWidget(entry)"
            :field="entry"
            :model-value="runValues[entry.key]"
            :i18n-namespace="plugin ? `plugin.${plugin.id}` : undefined"
            @update:model-value="(v: unknown) => (runValues[entry.key] = v)"
          />
        </template>
        <div
          v-if="visibleCommands.length > 0"
          class="run-form-buttons"
        >
          <button
            v-for="cmd in visibleCommands"
            :key="cmd.id"
            class="tool-btn primary"
            :disabled="running"
            @click="runCommand(cmd)"
          >
            <span v-if="runningCmd === cmd.id">
              {{ t('workspace.plugins.running') }}
            </span>
            <span v-else>{{ plugin ? commandLabel(plugin.id, cmd) : cmd.label || cmd.id }}</span>
          </button>
          <button
            v-if="running"
            class="tool-btn"
            type="button"
            :disabled="stopping"
            @click="stopRun"
          >
            {{ stopping ? t('workspace.plugins.stopping') : t('workspace.plugins.stop') }}
          </button>
        </div>
      </section>

      <template v-else>
        <section
          v-for="cmd in visibleCommands"
          :key="cmd.id"
          class="command-card"
        >
          <div class="command-header">
            <h3>{{ plugin ? commandLabel(plugin.id, cmd) : cmd.label || cmd.id }}</h3>
            <button
              class="tool-btn primary"
              :disabled="running"
              @click="runCommand(cmd)"
            >
              <span v-if="runningCmd === cmd.id">
                {{ t('workspace.plugins.running') }}
              </span>
              <span v-else>{{ t('workspace.plugins.run') }}</span>
            </button>
          </div>
        </section>
      </template>

      <PluginResultPanel
        :commands="visibleCommands"
        :results="runResults"
        :enabled="!!plugin.manifest.debug"
      />
    </div>

    <template #footer>
      <button class="tool-btn" type="button" @click="close">
        {{ t('common.close') }}
      </button>
    </template>
  </Modal>

  <StatChartDialog
    :open="chartOpen"
    :title="chartTitle"
    :charts="chartList"
    @close="closeChart"
  />
</template>
