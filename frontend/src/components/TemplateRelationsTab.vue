<script setup lang="ts">
/*
 * TemplateRelationsTab - the Relations setup tab. Self-contained: relations are
 * a sidecar (Relation service), NOT part of the template draft, so this owns its
 * own load + persist + editor + delete confirm. Reloads on template change and
 * when reloadKey bumps (after a global reconcile triggered in the parent).
 */
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import RelationEditorModal from "./RelationEditorModal.vue";
import ConfirmDialog from "./ConfirmDialog.vue";
import {
  Service as RelationSvc,
  Relation,
  Cardinality,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/relation";
import type { CardinalityOption } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/relation/models";
import { Service as DataproviderSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/dataprovider";
import type { TemplateSummary } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/dataprovider/models";
import { backendErrMessage } from "../utils/backendError";
import { useToast } from "../composables/useToast";

const props = defineProps<{ template: string; reloadKey?: number }>();

const { t } = useI18n();
const toast = useToast();

const relations = ref<Relation[]>([]);
const collectionTemplates = ref<TemplateSummary[]>([]);
const relationEditorOpen = ref(false);
const editingRelationIndex = ref(-1);
const editingRelation = ref<Relation>(new Relation({ to: "" }));

// Cardinality options + labels come from the backend; no frontend value->key map.
const cardinalityChoices = ref<CardinalityOption[]>([]);
void RelationSvc.Cardinalities().then((o) => (cardinalityChoices.value = o ?? []));
const cardinalityOptions = computed(() =>
  cardinalityChoices.value.map((o) => ({ value: o.value, label: t(o.label_key) })),
);
const defaultCardinality = computed<string>(
  () => cardinalityChoices.value.find((o) => o.default)?.value ?? "",
);
function cardinalityLabel(c: string): string {
  const opt = cardinalityChoices.value.find((o) => o.value === c);
  return opt ? t(opt.label_key) : c;
}
function relationTargetLabel(to: string): string {
  const s = collectionTemplates.value.find((x) => x.filename === to);
  return s?.name || s?.stem || to;
}

async function load() {
  relations.value = [];
  if (!props.template) return;
  try {
    collectionTemplates.value = await DataproviderSvc.ListCollectionTemplates();
    relations.value = (await RelationSvc.GetRelations(props.template)) ?? [];
  } catch (e) {
    relations.value = [];
    toast.error(backendErrMessage(e));
  }
}
watch(() => [props.template, props.reloadKey], () => void load(), { immediate: true });

// Editor target options: collection templates minus already-linked targets
// (except the one being edited), so a duplicate relation can't be picked.
const relationTargetOptions = computed(() => {
  const used = new Set(
    relations.value
      .filter((_, i) => i !== editingRelationIndex.value)
      .map((r) => r.to),
  );
  return collectionTemplates.value
    .filter((s) => !used.has(s.filename))
    .map((s) => ({
      value: s.filename,
      label:
        s.filename === props.template
          ? t("workspace.templates.relations.self_option", [s.name || s.stem])
          : s.name || s.stem,
    }));
});

function openAddRelation() {
  editingRelationIndex.value = -1;
  editingRelation.value = new Relation({
    to: "",
    cardinality: defaultCardinality.value as Cardinality,
  });
  relationEditorOpen.value = true;
}
function openEditRelation(idx: number) {
  const r = relations.value[idx];
  if (!r) return;
  editingRelationIndex.value = idx;
  editingRelation.value = new Relation({
    to: r.to,
    cardinality: r.cardinality,
    inverse: r.inverse,
  });
  relationEditorOpen.value = true;
}

// Immediate persist: write the whole set; revert on backend rejection.
async function persistRelations(next: Relation[]) {
  if (!props.template) return;
  const prev = relations.value;
  relations.value = next;
  try {
    await RelationSvc.SetRelations(props.template, next);
  } catch (e) {
    relations.value = prev;
    toast.error(backendErrMessage(e));
  }
}
function applyRelation(rel: Relation) {
  const next =
    editingRelationIndex.value < 0
      ? [...relations.value, rel]
      : relations.value.map((existing, i) =>
          i === editingRelationIndex.value ? rel : existing,
        );
  void persistRelations(next);
}

const confirmOpen = ref(false);
const pendingIndex = ref(-1);
const pendingLabel = computed(() => {
  const r = relations.value[pendingIndex.value];
  return r ? relationTargetLabel(r.to) : "";
});
function askRemove(idx: number) {
  pendingIndex.value = idx;
  confirmOpen.value = true;
}
function confirmRemove() {
  const idx = pendingIndex.value;
  confirmOpen.value = false;
  pendingIndex.value = -1;
  if (idx >= 0) void persistRelations(relations.value.filter((_, i) => i !== idx));
}
function cancelRemove() {
  confirmOpen.value = false;
  pendingIndex.value = -1;
}
</script>

<template>
  <div class="setup-tab-pane">
    <p class="muted small setup-tab-help">
      {{ t('workspace.templates.relations.help') }}
    </p>
    <p v-if="relations.length === 0" class="muted small">
      {{ t('workspace.templates.relations.empty') }}
    </p>
    <ul v-else class="relation-rows">
      <li
        v-for="(rel, i) in relations"
        :key="rel.to"
        class="relation-row list-card"
      >
        <span class="relation-row-target">{{ relationTargetLabel(rel.to) }}</span>
        <span
          v-if="rel.to === template"
          class="relation-row-inverse"
        >{{ t('workspace.templates.relations.self_label') }}</span>
        <span
          v-else-if="rel.inverse"
          class="relation-row-inverse"
        >{{ t('workspace.templates.relations.editor.inverse_label') }}</span>
        <code class="relation-row-cardinality mono">{{ cardinalityLabel(rel.cardinality) }}</code>
        <button
          class="tool-btn"
          type="button"
          :title="t('workspace.templates.relations.edit')"
          @click="openEditRelation(i)"
        >{{ t('workspace.templates.relations.edit') }}</button>
        <button
          class="tool-btn danger"
          type="button"
          :title="t('workspace.templates.relations.remove')"
          @click="askRemove(i)"
        >×</button>
      </li>
    </ul>
    <div class="setup-tab-actions">
      <button class="tool-btn" type="button" @click="openAddRelation">
        + {{ t('workspace.templates.relations.add') }}
      </button>
    </div>

    <RelationEditorModal
      :open="relationEditorOpen"
      :initial="editingRelation"
      :is-edit="editingRelationIndex >= 0"
      :targets="relationTargetOptions"
      :cardinalities="cardinalityOptions"
      @close="relationEditorOpen = false"
      @apply="applyRelation"
    />
    <ConfirmDialog
      :open="confirmOpen"
      :title="t('workspace.templates.relations.remove_title')"
      :message="t('workspace.templates.relations.remove_confirm', [pendingLabel])"
      :confirm-label="t('workspace.templates.relations.remove')"
      :cancel-label="t('common.cancel')"
      variant="danger"
      @confirm="confirmRemove"
      @cancel="cancelRemove"
    />
  </div>
</template>
