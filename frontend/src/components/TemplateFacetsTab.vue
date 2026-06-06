<script setup lang="ts">
/*
 * TemplateFacetsTab - the Facets setup tab, including the per-facet weighting
 * (scaling) subsystem. Presentational: the parent owns the draft; this renders
 * the facets list and emits update:facets / update:scalings. Owns the facet
 * editor, the weighting builder, and the delete confirm.
 */
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import draggable from "vuedraggable";
import FacetEditorModal from "./FacetEditorModal.vue";
import ScalingBuilderModal from "./ScalingBuilderModal.vue";
import FacetIcon from "./FacetIcon.vue";
import ConfirmDialog from "./ConfirmDialog.vue";
import {
  Facet,
  type Scaling,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { useFacetMeta } from "../composables/useFacetMeta";

const props = defineProps<{
  facets: Facet[];
  scalings: Scaling[];
}>();
const emit = defineEmits<{
  (e: "update:facets", v: Facet[]): void;
  (e: "update:scalings", v: Scaling[]): void;
}>();

const { t } = useI18n();

const { maxFacets, icons: facetIcons } = useFacetMeta();
const defaultFacetIcon = computed(() => facetIcons.value[0] ?? "fa-flag");

const facetsModel = computed<Facet[]>({
  get: () => props.facets ?? [],
  set: (v) => emit("update:facets", v),
});

// ── Weighting (scaling) per facet ────────────────────────────────────
const scalingBuilderOpen = ref(false);
const editingScalingIndex = ref(-1);
const editingScaling = ref<Scaling | null>(null);
const editingScalingFacet = ref<Facet | null>(null);

function scalingIndexForFacet(key: string): number {
  return (props.scalings ?? []).findIndex(
    (s) => s.source.kind === "facet" && s.source.key === key,
  );
}
function scalingForFacet(key: string): Scaling | null {
  const i = scalingIndexForFacet(key);
  return i >= 0 ? (props.scalings?.[i] ?? null) : null;
}
function openFacetWeighting(facet: Facet) {
  const idx = scalingIndexForFacet(facet.key);
  editingScalingIndex.value = idx;
  editingScaling.value = idx >= 0 ? (props.scalings?.[idx] ?? null) : null;
  editingScalingFacet.value = facet;
  scalingBuilderOpen.value = true;
}
function removeScaling(idx: number) {
  if (idx < 0) return;
  const cur = props.scalings ?? [];
  emit("update:scalings", [...cur.slice(0, idx), ...cur.slice(idx + 1)]);
}
function applyScaling(s: Scaling) {
  const cur = props.scalings ?? [];
  emit(
    "update:scalings",
    editingScalingIndex.value < 0
      ? [...cur, s]
      : cur.map((existing, i) => (i === editingScalingIndex.value ? s : existing)),
  );
  scalingBuilderOpen.value = false;
}
function onRemoveScaling() {
  removeScaling(editingScalingIndex.value);
  scalingBuilderOpen.value = false;
}

// ── Facet editor ─────────────────────────────────────────────────────
const facetEditorOpen = ref(false);
const editingFacetIndex = ref(-1);
const editingFacet = ref<Facet>(new Facet({ key: "", icon: "fa-flag", options: [] }));

function openAddFacet() {
  if ((props.facets?.length ?? 0) >= maxFacets.value) return;
  editingFacetIndex.value = -1;
  editingFacet.value = new Facet({ key: "", icon: defaultFacetIcon.value, options: [] });
  facetEditorOpen.value = true;
}
function openEditFacet(idx: number) {
  const f = props.facets?.[idx];
  if (!f) return;
  editingFacetIndex.value = idx;
  editingFacet.value = new Facet({
    key: f.key,
    icon: f.icon,
    options: (f.options ?? []).map((o) => ({ label: o.label, color: o.color })),
  });
  facetEditorOpen.value = true;
}
function applyFacet(f: Facet) {
  const cur = props.facets ?? [];
  emit(
    "update:facets",
    editingFacetIndex.value < 0
      ? [...cur, f]
      : cur.map((existing, i) => (i === editingFacetIndex.value ? f : existing)),
  );
}
const otherFacetKeys = computed(() =>
  (props.facets ?? [])
    .filter((_, i) => i !== editingFacetIndex.value)
    .map((f) => f.key),
);

// ── Delete confirm ───────────────────────────────────────────────────
const confirmOpen = ref(false);
const pendingIndex = ref(-1);
const pendingKey = computed(() => props.facets?.[pendingIndex.value]?.key ?? "");
function askRemove(idx: number) {
  pendingIndex.value = idx;
  confirmOpen.value = true;
}
function confirmRemove() {
  const idx = pendingIndex.value;
  confirmOpen.value = false;
  pendingIndex.value = -1;
  if (idx >= 0) {
    const cur = props.facets ?? [];
    emit("update:facets", [...cur.slice(0, idx), ...cur.slice(idx + 1)]);
  }
}
function cancelRemove() {
  confirmOpen.value = false;
  pendingIndex.value = -1;
}
</script>

<template>
  <div class="setup-tab-pane">
    <p v-if="facetsModel.length === 0" class="muted small">
      {{ t('workspace.templates.facets.summary_empty') }}
    </p>
    <draggable
      v-else
      v-model="facetsModel"
      tag="ul"
      class="facet-rows"
      handle=".dnd-handle"
      :animation="150"
      ghost-class="dnd-ghost"
      chosen-class="dnd-chosen"
      drag-class="dnd-drag"
      item-key="key"
    >
      <template #item="{ element: f, index: i }">
        <li class="facet-row list-card">
          <span class="dnd-handle" aria-hidden="true">☰</span>
          <FacetIcon :icon="f.icon" class="facet-row-icon" />
          <span class="facet-row-key mono">{{ f.key }}</span>
          <span class="muted small facet-row-summary">
            {{ t('workspace.templates.facets.options_count', [f.options.length]) }}
          </span>
          <code
            v-if="scalingForFacet(f.key)"
            class="facet-row-weighting mono"
            :title="t('workspace.templates.scalings.intro')"
          >S["{{ scalingForFacet(f.key)?.name }}"]</code>
          <button
            class="tool-btn"
            type="button"
            :title="t('workspace.templates.facets.edit')"
            @click="openEditFacet(i)"
          >{{ t('workspace.templates.facets.edit') }}</button>
          <button
            class="tool-btn"
            type="button"
            :class="{ 'is-active': !!scalingForFacet(f.key) }"
            :title="t('workspace.templates.scalings.edit')"
            @click="openFacetWeighting(f)"
          >{{ t('workspace.templates.scalings.button') }}</button>
          <button
            class="tool-btn danger"
            type="button"
            :title="t('workspace.templates.facets.remove')"
            @click="askRemove(i)"
          >×</button>
        </li>
      </template>
    </draggable>
    <div class="setup-tab-actions">
      <span class="muted small">
        {{ t('workspace.templates.facets.counter', [facetsModel.length, maxFacets]) }}
      </span>
      <button
        class="tool-btn"
        type="button"
        :disabled="facetsModel.length >= maxFacets"
        @click="openAddFacet"
      >+ {{ t('workspace.templates.facets.add') }}</button>
    </div>
    <p class="muted small facets-weighting-hint">
      {{ t('workspace.templates.scalings.intro') }}
    </p>

    <FacetEditorModal
      :open="facetEditorOpen"
      :initial="editingFacet"
      :existing-keys="otherFacetKeys"
      @close="facetEditorOpen = false"
      @apply="applyFacet"
    />
    <ScalingBuilderModal
      :open="scalingBuilderOpen"
      :facet="editingScalingFacet"
      :initial="editingScaling"
      @close="scalingBuilderOpen = false"
      @apply="applyScaling"
      @remove="onRemoveScaling"
    />
    <ConfirmDialog
      :open="confirmOpen"
      :title="t('workspace.templates.facets.remove_title')"
      :message="t('workspace.templates.facets.remove_confirm', [pendingKey])"
      :confirm-label="t('workspace.templates.facets.remove')"
      :cancel-label="t('common.cancel')"
      variant="danger"
      @confirm="confirmRemove"
      @cancel="cancelRemove"
    />
  </div>
</template>
