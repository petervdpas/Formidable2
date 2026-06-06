<script setup lang="ts">
/*
 * RelationLinksPanel - per-record edge linking, rendered inside the Storage
 * sidebar's Relations popover. For the open record it shows, per relation its
 * template declares, the records it's linked to (with remove) and a picker over
 * the target collection's records (add). Edges persist immediately and mirror to
 * the other side via the Relation service; this is NOT part of the form draft.
 */
import { onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
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
      <div class="relation-links-head">
        <span class="relation-links-target">{{ templateName(r.to) }}</span>
        <span
          v-if="r.inverse"
          class="relation-row-inverse"
        >{{ t('workspace.templates.relations.editor.inverse_label') }}</span>
      </div>
      <ul
        v-if="linkedIds(r).length"
        class="relation-links-chips"
      >
        <li
          v-for="id in linkedIds(r)"
          :key="id"
          class="relation-links-chip"
        >
          <span class="relation-links-chip-label">{{ itemTitle(r.to, id) }}</span>
          <button
            class="tool-btn danger"
            type="button"
            :title="t('workspace.storage.relations.remove')"
            @click="removeLink(r, id)"
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
    </div>
  </div>
</template>
