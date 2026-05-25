<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import SelectField from "../fields/SelectField.vue";
import StatChart from "../stat/StatChart.vue";
import { extractCharts, type StatResult } from "../stat/types";
import {
  Service as StatSvc,
  type StatObject,
  type ChartShapeDescriptor,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/stat";
import {
  Service as PluginSvc,
  type ListResult,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";
import type { Widget } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/formwidget";
import {
  useGlobalPluginRun,
  setGlobalPluginRunning,
} from "../../composables/useGlobalPluginRun";
import { useToast } from "../../composables/useToast";

// ChartWidget - the interactive statistics widget that lives inside a
// form (run_mode "form"). It owns a statistical-object picker
// (Stat.ListObjects on the active template) and a chart-shape picker
// (Stat.ChartShapes - both backend-owned), runs the plugin's command
// with the selection as ctx, and renders the returned chart envelope.
// Unlike ProgressBar/StatusMessage widgets it drives the Lua call
// itself rather than being fed by a run-scoped event.
const props = defineProps<{
  widget: Widget;
  plugin: ListResult;
  template: string;
}>();

const { t } = useI18n();
const toast = useToast();
const { running } = useGlobalPluginRun();

const objects = ref<StatObject[]>([]);
const shapes = ref<ChartShapeDescriptor[]>([]);
const selectedObject = ref<string>("");
const selectedShape = ref<string>("");
const result = ref<StatResult | null>(null);
const resultType = ref<string>("");

const objectOptions = computed(() =>
  objects.value.map((o) => ({ value: o.name, label: o.label || o.name })),
);
const shapeOptions = computed(() =>
  shapes.value.map((s) => ({ value: s.name, label: t(s.label_key) })),
);

// The plugin's first command is the one this widget asks Lua to run;
// it receives {template, object, shape} and returns a chart envelope.
const command = computed(() => props.plugin.manifest.commands?.[0] ?? null);

async function loadParams(): Promise<void> {
  result.value = null;
  resultType.value = "";
  try {
    const [objs, shp] = await Promise.all([
      StatSvc.ListObjects(props.template),
      StatSvc.ChartShapes(),
    ]);
    objects.value = objs ?? [];
    shapes.value = shp ?? [];
    selectedObject.value = objects.value[0]?.name ?? "";
    selectedShape.value = shapes.value[0]?.name ?? "";
  } catch {
    objects.value = [];
    shapes.value = [];
  }
}

async function draw(): Promise<void> {
  const cmd = command.value;
  if (!cmd || !selectedObject.value || !selectedShape.value) return;
  if (running.value) return;
  setGlobalPluginRunning(true);
  try {
    const res = await PluginSvc.Run(props.plugin.id, cmd.id, {
      template: props.template,
      object: selectedObject.value,
      shape: selectedShape.value,
    });
    if (res.kind === "ok") {
      const first = extractCharts(res.value)[0];
      result.value = first?.result ?? null;
      resultType.value = first?.type || selectedShape.value;
    } else if (res.kind === "busy") {
      toast.warn(res.message || "");
    }
    for (const ev of res.toasts ?? []) {
      const fn = toast[ev.level as "info" | "success" | "warn" | "error"];
      if (fn) fn(ev.message);
    }
  } catch (err) {
    toast.error(String(err));
  } finally {
    setGlobalPluginRunning(false);
  }
}

watch(() => props.template, () => void loadParams(), { immediate: true });
watch([selectedObject, selectedShape], () => void draw());
</script>

<template>
  <div class="form-widget form-widget-chart">
    <label v-if="widget.label" class="form-widget-label">{{ widget.label }}</label>
    <div class="chart-widget">
      <div class="chart-widget-params">
        <label class="chart-widget-field">
          <span class="small muted">{{ t('workspace.plugins.chart.object') }}</span>
          <SelectField
            v-model="selectedObject"
            :options="objectOptions"
            :disabled="running || objectOptions.length === 0"
            :placeholder="t('workspace.plugins.chart.object_placeholder')"
          />
        </label>
        <label class="chart-widget-field">
          <span class="small muted">{{ t('workspace.plugins.chart.shape') }}</span>
          <SelectField
            v-model="selectedShape"
            :options="shapeOptions"
            :disabled="running || shapeOptions.length === 0"
          />
        </label>
        <p v-if="objectOptions.length === 0" class="muted small">
          {{ t('workspace.plugins.chart.no_objects') }}
        </p>
      </div>
      <div class="chart-widget-display">
        <StatChart v-if="result" :result="result" :type="resultType" />
        <p v-else class="muted small chart-widget-empty">
          {{ t('workspace.plugins.chart.no_data') }}
        </p>
      </div>
    </div>
  </div>
</template>
