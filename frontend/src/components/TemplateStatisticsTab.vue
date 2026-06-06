<script setup lang="ts">
/*
 * TemplateStatisticsTab - the Statistics setup tab: DSL statistics + composite
 * (hop route) objects + the evaluated-grid viewer. Presentational: the parent
 * owns the draft; this renders the statistics list and emits update:statistics.
 * Owns its builder/composite/grid modals and delete confirm.
 */
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import draggable from "vuedraggable";
import StatisticsBuilderModal from "./StatisticsBuilderModal.vue";
import CompositeBuilderModal from "./CompositeBuilderModal.vue";
import StatGridDialog from "./stat/StatGridDialog.vue";
import ConfirmDialog from "./ConfirmDialog.vue";
import { type Grid, type CompositeGrid } from "./stat/grid";
import {
  Statistic,
  type Field,
  type Facet,
  type Formula,
  type Scaling,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { Service as StatSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/stat";
import type { CompositeSpec } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/stat/models";
import { backendErrMessage } from "../utils/backendError";
import { useToast } from "../composables/useToast";

const props = defineProps<{
  template: string;
  statistics: Statistic[];
  fields: Field[];
  facets: Facet[];
  formulas: Formula[];
  scalings: Scaling[];
}>();
const emit = defineEmits<{ (e: "update:statistics", v: Statistic[]): void }>();

const { t } = useI18n();
const toast = useToast();

const statisticsModel = computed<Statistic[]>({
  get: () => props.statistics ?? [],
  set: (v) => emit("update:statistics", v),
});

// editingIndex steers insert-vs-replace for both the DSL and composite builders.
const statBuilderOpen = ref(false);
const editingIndex = ref(-1);
const editingStat = ref<Statistic | null>(null);
const compositeBuilderOpen = ref(false);
const editingComposite = ref<Statistic | null>(null);

function openAddStatistic() {
  editingIndex.value = -1;
  editingStat.value = null;
  statBuilderOpen.value = true;
}
function openEditStatistic(idx: number) {
  const s = props.statistics?.[idx];
  if (!s) return;
  editingIndex.value = idx;
  if (s.composite) {
    editingComposite.value = s;
    compositeBuilderOpen.value = true;
    return;
  }
  editingStat.value = new Statistic({ name: s.name, label: s.label, dsl: s.dsl });
  statBuilderOpen.value = true;
}
function applyStatistic(s: Statistic) {
  const cur = props.statistics ?? [];
  emit(
    "update:statistics",
    editingIndex.value < 0
      ? [...cur, s]
      : cur.map((existing, i) => (i === editingIndex.value ? s : existing)),
  );
  statBuilderOpen.value = false;
}
function openAddComposite() {
  editingIndex.value = -1;
  editingComposite.value = null;
  compositeBuilderOpen.value = true;
}
function applyComposite(s: Statistic) {
  applyStatistic(s);
  compositeBuilderOpen.value = false;
}

// Evaluated-grid viewer: evaluates the draft's current DSL/spec so it works on
// unsaved edits (it only reads the template's already-indexed values).
const statViewOpen = ref(false);
const statViewGrid = ref<Grid | CompositeGrid | null>(null);
const statViewTitle = ref("");
async function openViewStatistic(s: Statistic) {
  if (!props.template) return;
  try {
    const grid = s.composite
      ? await StatSvc.EvaluateCompositeSpec(props.template, s.composite as unknown as CompositeSpec)
      : await StatSvc.EvaluateDSL(props.template, s.dsl);
    statViewGrid.value = grid as unknown as Grid | CompositeGrid;
    statViewTitle.value = s.label || s.name;
    statViewOpen.value = true;
  } catch (e) {
    toast.error("workspace.templates.statistics.view_failed", [backendErrMessage(e)]);
  }
}

const confirmOpen = ref(false);
const pendingIndex = ref(-1);
const pendingLabel = computed(() => {
  const s = props.statistics?.[pendingIndex.value];
  return s ? s.label || s.name : "";
});
function askRemove(idx: number) {
  pendingIndex.value = idx;
  confirmOpen.value = true;
}
function confirmRemove() {
  const idx = pendingIndex.value;
  confirmOpen.value = false;
  pendingIndex.value = -1;
  if (idx >= 0) {
    const cur = props.statistics ?? [];
    emit("update:statistics", [...cur.slice(0, idx), ...cur.slice(idx + 1)]);
  }
}
function cancelRemove() {
  confirmOpen.value = false;
  pendingIndex.value = -1;
}
</script>

<template>
  <div class="setup-tab-pane">
    <p v-if="statisticsModel.length === 0" class="muted small">
      {{ t('workspace.templates.statistics.empty') }}
    </p>
    <draggable
      v-else
      v-model="statisticsModel"
      tag="ul"
      class="stat-rows"
      handle=".dnd-handle"
      :animation="150"
      ghost-class="dnd-ghost"
      chosen-class="dnd-chosen"
      drag-class="dnd-drag"
      item-key="name"
    >
      <template #item="{ element: s, index: i }">
        <li class="stat-row list-card">
          <span class="dnd-handle" aria-hidden="true">☰</span>
          <span class="stat-row-name">{{ s.label || s.name }}</span>
          <code v-if="s.composite" class="stat-row-dsl">{{ t('workspace.templates.statistics.composite_summary', [s.composite.parent, s.composite.edges.length]) }}</code>
          <code v-else class="stat-row-dsl">{{ s.dsl }}</code>
          <button
            class="tool-btn"
            type="button"
            :title="t('workspace.templates.statistics.view')"
            @click="openViewStatistic(s)"
          >{{ t('workspace.templates.statistics.view') }}</button>
          <button
            class="tool-btn"
            type="button"
            :title="t('workspace.templates.statistics.edit')"
            @click="openEditStatistic(i)"
          >{{ t('workspace.templates.statistics.edit') }}</button>
          <button
            class="tool-btn danger"
            type="button"
            :title="t('workspace.templates.statistics.remove')"
            @click="askRemove(i)"
          >×</button>
        </li>
      </template>
    </draggable>
    <div class="setup-tab-actions">
      <button class="tool-btn" type="button" @click="openAddStatistic">
        + {{ t('workspace.templates.statistics.add') }}
      </button>
      <button class="tool-btn" type="button" @click="openAddComposite">
        + {{ t('workspace.templates.statistics.add_composite') }}
      </button>
    </div>

    <StatisticsBuilderModal
      :open="statBuilderOpen"
      :template="template"
      :fields="fields"
      :facets="facets"
      :formulas="formulas"
      :scalings="scalings"
      :initial="editingStat"
      @close="statBuilderOpen = false"
      @apply="applyStatistic"
    />
    <CompositeBuilderModal
      :open="compositeBuilderOpen"
      :template="template"
      :statistics="statistics"
      :initial="editingComposite"
      @close="compositeBuilderOpen = false"
      @apply="applyComposite"
    />
    <StatGridDialog
      :open="statViewOpen"
      :title="statViewTitle"
      :grid="statViewGrid"
      :facets="facets"
      @close="statViewOpen = false"
    />
    <ConfirmDialog
      :open="confirmOpen"
      :title="t('workspace.templates.statistics.remove_title')"
      :message="t('workspace.templates.statistics.remove_confirm', [pendingLabel])"
      :confirm-label="t('workspace.templates.statistics.remove')"
      :cancel-label="t('common.cancel')"
      variant="danger"
      @confirm="confirmRemove"
      @cancel="cancelRemove"
    />
  </div>
</template>
