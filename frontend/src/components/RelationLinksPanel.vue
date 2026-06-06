<script setup lang="ts">
/*
 * RelationLinksPanel - per-record edge linking, rendered inside the Storage
 * sidebar's Relations popover. For the open record it shows, per relation its
 * template declares, the records it's linked to (with remove) and a picker over
 * the target collection's records (add). Edges persist immediately and mirror to
 * the other side via the Relation service; this is NOT part of the form draft.
 */
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import ConfirmDialog from "./ConfirmDialog.vue";
import {
  Service as RelationSvc,
  Relation,
  Edge,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/relation";
import { Service as DataproviderSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/dataprovider";
import type { CollectionItem } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/dataprovider/models";
import { SelectField } from "./fields";
import { backendErrMessage } from "../utils/backendError";
import { useToast } from "../composables/useToast";

const props = defineProps<{ template: string; recordId: string }>();

const { t } = useI18n();
const toast = useToast();

const relations = ref<Relation[]>([]);
const targetItems = ref<Record<string, CollectionItem[]>>({});
const templateNames = ref<Record<string, string>>({});
const busy = ref(false);

// Each relation group is collapsed by default; expanding shows its links + picker.
const expanded = ref<Set<string>>(new Set());
function isExpanded(to: string): boolean {
  return expanded.value.has(to);
}
function toggleGroup(to: string) {
  const next = new Set(expanded.value);
  next.has(to) ? next.delete(to) : next.add(to);
  expanded.value = next;
}

async function load() {
  if (!props.template || !props.recordId) return;
  busy.value = true;
  try {
    relations.value = (await RelationSvc.GetRelations(props.template)) ?? [];
    const tpls = (await DataproviderSvc.ListCollectionTemplates()) ?? [];
    const names: Record<string, string> = {};
    for (const s of tpls) names[s.filename] = s.name || s.stem;
    templateNames.value = names;
    const items: Record<string, CollectionItem[]> = {};
    for (const r of relations.value) {
      if (!items[r.to]) {
        items[r.to] = (await DataproviderSvc.ListCollectionItems(r.to)) ?? [];
      }
    }
    targetItems.value = items;
  } catch (e) {
    toast.error(backendErrMessage(e));
  } finally {
    busy.value = false;
  }
}
onMounted(load);

function linkedIds(r: Relation): string[] {
  return (r.edges ?? [])
    .filter((e) => e.from === props.recordId)
    .map((e) => e.to);
}
function itemTitle(to: string, id: string): string {
  return (targetItems.value[to] ?? []).find((x) => x.id === id)?.title || id;
}
function templateName(to: string): string {
  return templateNames.value[to] || to;
}
function addOptions(r: Relation): { value: string; label: string }[] {
  const linked = new Set(linkedIds(r));
  return (targetItems.value[r.to] ?? [])
    .filter((x) => x.id && !linked.has(x.id) && x.id !== props.recordId)
    .map((x) => ({ value: x.id, label: x.title || x.id }));
}

async function addLink(r: Relation, targetId: string) {
  if (!targetId) return;
  try {
    await RelationSvc.AddEdge(
      props.template,
      r.to,
      new Edge({ from: props.recordId, to: targetId }),
    );
    await load();
  } catch (e) {
    toast.error(backendErrMessage(e));
  }
}
async function removeLink(r: Relation, targetId: string) {
  try {
    await RelationSvc.RemoveEdge(
      props.template,
      r.to,
      new Edge({ from: props.recordId, to: targetId }),
    );
    await load();
  } catch (e) {
    toast.error(backendErrMessage(e));
  }
}

// Removal is confirmed first; the dialog is a modal over the popover, which stays open.
const confirmOpen = ref(false);
const pending = ref<{ relation: Relation; id: string } | null>(null);
const pendingTitle = computed(() =>
  pending.value ? itemTitle(pending.value.relation.to, pending.value.id) : "",
);
function askRemove(relation: Relation, id: string) {
  pending.value = { relation, id };
  confirmOpen.value = true;
}
function cancelRemove() {
  confirmOpen.value = false;
  pending.value = null;
}
async function confirmRemove() {
  const p = pending.value;
  confirmOpen.value = false;
  pending.value = null;
  if (p) await removeLink(p.relation, p.id);
}
</script>

<template>
  <div class="relation-links">
    <p v-if="relations.length === 0" class="muted small">
      {{ t('workspace.storage.relations.no_relations') }}
    </p>
    <div
      v-for="r in relations"
      :key="r.to"
      class="relation-links-group"
    >
      <button
        type="button"
        class="relation-links-head relation-links-toggle"
        :aria-expanded="isExpanded(r.to)"
        @click="toggleGroup(r.to)"
      >
        <i
          class="fa-solid relation-links-caret"
          :class="isExpanded(r.to) ? 'fa-chevron-down' : 'fa-chevron-right'"
          aria-hidden="true"
        ></i>
        <span class="relation-links-target">{{ templateName(r.to) }}</span>
        <span
          v-if="r.to === template"
          class="relation-row-inverse"
        >{{ t('workspace.templates.relations.self_label') }}</span>
        <span
          v-else-if="r.inverse"
          class="relation-row-inverse"
        >{{ t('workspace.templates.relations.editor.inverse_label') }}</span>
        <span class="relation-links-count">{{ linkedIds(r).length }}</span>
      </button>
      <template v-if="isExpanded(r.to)">
        <ul
          v-if="linkedIds(r).length"
          class="relation-links-chips"
        >
          <li
            v-for="id in linkedIds(r)"
            :key="id"
            class="relation-links-chip list-card"
          >
            <span class="relation-links-chip-label">{{ itemTitle(r.to, id) }}</span>
            <button
              class="tool-btn danger"
              type="button"
              :title="t('workspace.storage.relations.remove')"
              @click="askRemove(r, id)"
            >×</button>
          </li>
        </ul>
        <p v-else class="muted small">
          {{ t('workspace.storage.relations.no_links') }}
        </p>
        <SelectField
          :model-value="''"
          :options="addOptions(r)"
          :placeholder="t('workspace.storage.relations.add_link')"
          @update:model-value="(v: string) => addLink(r, v)"
        />
      </template>
    </div>

    <ConfirmDialog
      :open="confirmOpen"
      elevated
      :title="t('workspace.storage.relations.remove_title')"
      :message="t('workspace.storage.relations.remove_confirm', [pendingTitle])"
      :confirm-label="t('workspace.storage.relations.remove')"
      :cancel-label="t('common.cancel')"
      variant="danger"
      @confirm="confirmRemove"
      @cancel="cancelRemove"
    />
  </div>
</template>
