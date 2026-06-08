<script setup lang="ts">
// Form-side renderer for an api (relation reference) field. The field stores
// only the referenced id(s); the target columns are read live. Single vs multi
// comes from the declared relation's cardinality (backend-owned: the relation's
// CardinalityOption.source_many). Actions: link an existing target, create a new
// one (first-class, owned by the target), unlink, and go to the target to finish
// its other fields. Relation edges are reconciled by the backend on host save,
// so this component only edits the field value.
//
// modelValue: a string id (single), a string[] of ids (to-many), or null/"".

import { computed, inject, ref, watch, type Ref } from "vue";
import { useI18n } from "vue-i18n";
import draggable from "vuedraggable";
import APIFieldPicker from "./APIFieldPicker.vue";
import ReferenceCreateDialog from "./ReferenceCreateDialog.vue";
import {
  Service as DataproviderSvc,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/dataprovider";
import type { CollectionItem } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/dataprovider/models";
import {
  Service as RelationSvc,
  type Relation,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/relation";
import {
  Service as TemplateSvc,
  type Field,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { useConfig } from "../../composables/useConfig";
import { useFormidableLink } from "../../composables/useFormidableLink";
import { backendErrMessage } from "../../utils/backendError";
import { useToast } from "../../composables/useToast";

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const { t } = useI18n();
const toast = useToast();
const formidableLink = useFormidableLink();

// Host template filename, provided by StorageWorkspace; needed to find the
// declared relation (for cardinality). Absent (e.g. plugin workspace) -> inert.
const hostTemplate = inject<Ref<string>>("templateFilename", ref(""));

const pickerOpen = ref(false);
const createOpen = ref(false);

// ── Field config ───────────────────────────────────────────────────────
const collection = computed(() => props.field.collection ?? "");
const mapKeys = computed<string[]>(() =>
  (props.field.map ?? []).map((m) => m.key).filter(Boolean),
);

const ids = computed<string[]>(() => {
  const v = props.modelValue;
  if (typeof v === "string") return v ? [v] : [];
  if (Array.isArray(v)) return v.filter((x): x is string => typeof x === "string" && x !== "");
  return [];
});

// ── Declared relation + cardinality (backend-driven single/multi) ──────
const relation = ref<Relation | null>(null);
const multi = ref(false);
const declared = computed(() => relation.value !== null);

async function loadRelation() {
  relation.value = null;
  multi.value = false;
  const host = hostTemplate.value;
  if (!host || !collection.value) return;
  try {
    const rels = (await RelationSvc.GetRelations(host)) ?? [];
    const rel = rels.find((r) => r.to === collection.value) ?? null;
    relation.value = rel;
    if (rel) {
      const opts = (await RelationSvc.Cardinalities()) ?? [];
      multi.value = opts.find((o) => o.value === rel.cardinality)?.source_many ?? false;
    }
  } catch (e) {
    toast.error(backendErrMessage(e));
  }
}

watch([() => hostTemplate.value, collection], () => void loadRelation(), {
  immediate: true,
});

// ── Target collection items: id -> {title, filename} ───────────────────
const items = ref<Record<string, CollectionItem>>({});

async function loadItems() {
  items.value = {};
  if (!collection.value) return;
  try {
    const list = (await DataproviderSvc.ListCollectionItems(collection.value)) ?? [];
    const map: Record<string, CollectionItem> = {};
    for (const it of list) if (it.id) map[it.id] = it;
    items.value = map;
  } catch {
    items.value = {};
  }
}

watch(collection, () => void loadItems(), { immediate: true });

// ── Live target columns: id -> row(keyed by map key) ───────────────────
const rows = ref<Record<string, Record<string, any>>>({});

async function loadRows() {
  if (!collection.value || mapKeys.value.length === 0) return;
  for (const id of ids.value) {
    if (rows.value[id]) continue;
    try {
      const res = await DataproviderSvc.FetchAPIFieldRow(collection.value, id, mapKeys.value);
      if (!res.kind) rows.value[id] = res.row ?? {};
    } catch {
      /* volatile: a since-deleted target just shows no columns */
    }
  }
}

watch([ids, collection, mapKeys], () => void loadRows(), { immediate: true });

// ── Target field types (for rich column rendering) ─────────────────────
const sourceFields = ref<Record<string, Field>>({});

async function loadSourceFields() {
  sourceFields.value = {};
  if (!collection.value) return;
  try {
    const tpl = await TemplateSvc.LoadTemplate(collection.value);
    const map: Record<string, Field> = {};
    for (const f of tpl?.fields ?? []) if (f.key) map[f.key] = f;
    sourceFields.value = map;
  } catch {
    sourceFields.value = {};
  }
}

watch(collection, () => void loadSourceFields(), { immediate: true });

// ── Value mutators (edges are reconciled by the backend on host save) ──
function emitIds(next: string[]) {
  const deduped = Array.from(new Set(next));
  emit("update:modelValue", multi.value ? deduped : (deduped[0] ?? ""));
}

// Reorder via drag/drop. Collapse state is keyed by id, so it travels with the
// card automatically; only the id order changes.
const draggableIds = computed<string[]>({
  get: () => ids.value,
  set: (next) => emitIds(next),
});
const dndScope = computed(() => `api-${props.field.key}`);

// Block a drag attempt on an expanded card at the mousedown layer (same rule as
// the loop: only collapsed cards drag), with a toast hint.
function onHandleMousedown(e: MouseEvent, id: string) {
  if (!isExpanded(id)) return;
  e.preventDefault();
  e.stopPropagation();
  toast.warn("workspace.storage.field.drag_collapse_first");
}
function addId(id: string) {
  if (!id) return;
  emitIds(multi.value ? [...ids.value, id] : [id]);
}
function removeId(id: string) {
  emitIds(ids.value.filter((x) => x !== id));
}

function replaceId(oldId: string, newId: string) {
  if (!newId) return;
  emitIds(ids.value.map((x) => (x === oldId ? newId : x)));
}

// Per-card collapse state, keyed by id. The default (before any manual toggle)
// follows the Auto-collapse Fields setting; a manual toggle overrides it.
const { config } = useConfig();
const defaultExpanded = computed(() => !(config.value?.field_state_collapsed ?? true));
const expanded = ref<Record<string, boolean>>({});
function isExpanded(id: string): boolean {
  return expanded.value[id] ?? defaultExpanded.value;
}
function toggleCard(id: string) {
  expanded.value = { ...expanded.value, [id]: !isExpanded(id) };
}

// When set, the next pick swaps this existing link instead of appending.
const editingId = ref<string | null>(null);

function openPicker() {
  editingId.value = null;
  pickerOpen.value = true;
}
function changeLink(id: string) {
  editingId.value = id;
  pickerOpen.value = true;
}
function closePicker() {
  pickerOpen.value = false;
  editingId.value = null;
}

function onPick(id: string) {
  pickerOpen.value = false;
  if (editingId.value) {
    replaceId(editingId.value, id);
    editingId.value = null;
  } else {
    addId(id);
  }
}
function onCreated(id: string) {
  addId(id);
}

async function goTo(id: string) {
  // Backend builds the canonical formidable:// link (same builder the rendered
  // card uses); follow() pushes nav history and honors the unsaved guard.
  const res = await DataproviderSvc.ResolveAPIFieldLink(collection.value, id);
  if (res?.kind || !res?.href) {
    toast.error(res?.message || "status.template.load.failed");
    return;
  }
  await formidableLink.follow(res.href);
}

function titleFor(id: string): string {
  return items.value[id]?.title || id;
}
function rowLabel(idx: number, key: string): string {
  return props.field.map?.[idx]?.label?.trim() || key;
}

// Footer action labels. Multi (to-many) adds another link / record; single
// replaces the one link. Empty in either mode just reads "Link…".
const linkLabel = computed(() => {
  if (multi.value && ids.value.length)
    return t("workspace.storage.api_field.link_another");
  if (!multi.value && ids.value.length)
    return t("workspace.storage.api_field.relink");
  return t("workspace.storage.api_field.link");
});
const createLabel = computed(() =>
  multi.value && ids.value.length
    ? t("workspace.storage.api_field.create_another")
    : t("workspace.storage.api_field.create"),
);

// ── Rich column rendering (mirrors the source field's type) ─────────────
function display(v: any): string {
  if (v == null) return "";
  if (typeof v === "object") return JSON.stringify(v);
  return String(v);
}

type RenderedShape =
  | { kind: "text"; text: string }
  | { kind: "tags"; items: string[] }
  | { kind: "list"; items: any[] }
  | { kind: "table"; headers: string[] | null; rows: any[][] };

function shapeFor(raw: any, sourceField: Field | undefined): RenderedShape {
  if (raw == null) return { kind: "text", text: "" };
  const type = sourceField?.type ?? "";
  if (type === "tags" && Array.isArray(raw)) return { kind: "tags", items: raw.map(String) };
  if (type === "list" && Array.isArray(raw)) return { kind: "list", items: raw };
  if ((type === "table" || type === "multioption") && Array.isArray(raw)) {
    if (raw.length === 0) return { kind: "text", text: "" };
    return shapeTable(raw, sourceField);
  }
  return { kind: "text", text: display(raw) };
}

function headersForTableField(field: Field | undefined): string[] | null {
  const opts = field?.options;
  if (!Array.isArray(opts) || opts.length === 0) return null;
  const headers: string[] = [];
  for (const o of opts) {
    if (o && typeof o === "object") {
      const rec = o as Record<string, unknown>;
      const label =
        typeof rec.label === "string" && rec.label.trim() !== ""
          ? rec.label
          : typeof rec.value === "string"
            ? rec.value
            : "";
      headers.push(label);
    } else {
      headers.push("");
    }
  }
  return headers;
}

function shapeTable(rowsIn: any[], sourceField: Field | undefined): RenderedShape {
  const first = rowsIn[0];
  if (Array.isArray(first)) {
    const headers = headersForTableField(sourceField);
    const arity = headers?.length ?? Math.max(...rowsIn.map((r) => (Array.isArray(r) ? r.length : 0)));
    return {
      kind: "table",
      headers,
      rows: rowsIn.map((r) => {
        const arr = Array.isArray(r) ? r.slice() : [];
        while (arr.length < arity) arr.push("");
        return arr.slice(0, arity);
      }),
    };
  }
  if (first && typeof first === "object") {
    const headers = headersForTableField(sourceField);
    const keys = headers
      ? ((sourceField?.options ?? []) as Array<Record<string, unknown>>).map((o) =>
          o && typeof o === "object" ? String(o.value ?? "") : "",
        )
      : Object.keys(first);
    return {
      kind: "table",
      headers: headers ?? keys,
      rows: rowsIn.map((r) => keys.map((k) => (r as any)?.[k])),
    };
  }
  return { kind: "table", headers: null, rows: rowsIn.map((r) => [r]) };
}
</script>

<template>
  <div class="api-field">
    <!-- No declared relation (or no host context): the field is inert. -->
    <p v-if="!collection || (hostTemplate && !declared)" class="muted small">
      {{ t('workspace.storage.api_field.no_relation') }}
    </p>

    <template v-else>
      <!-- Linked record cards (drag to reorder; only when collapsed) -->
      <draggable
        v-if="ids.length"
        v-model="draggableIds"
        tag="div"
        class="api-field-cards"
        :data-dnd-scope="dndScope"
        :group="dndScope"
        handle=".dnd-handle"
        :animation="150"
        ghost-class="dnd-ghost"
        chosen-class="dnd-chosen"
        drag-class="dnd-drag"
        :item-key="(id: string) => id"
      >
        <template #item="{ element: id }">
        <section
          class="api-field-card"
          :class="{ collapsed: !isExpanded(id) }"
        >
          <header class="api-field-card-head">
            <span
              class="dnd-handle"
              :class="{ disabled: isExpanded(id) }"
              :title="isExpanded(id)
                ? t('workspace.storage.field.drag_collapse_first')
                : t('workspace.storage.field.drag_to_reorder')"
              aria-hidden="true"
              @mousedown="onHandleMousedown($event, id)"
            >⠿</span>
            <button
              type="button"
              class="btn-ghost-icon btn-sm"
              :aria-expanded="isExpanded(id)"
              @click="toggleCard(id)"
            >{{ isExpanded(id) ? '▼' : '▶' }}</button>
            <span class="api-field-card-title" @click="toggleCard(id)">{{ titleFor(id) }}</span>
            <span v-if="isExpanded(id)" class="api-field-actions">
              <button
                type="button"
                class="tool-btn small"
                @click="goTo(id)"
                :title="t('workspace.storage.api_field.go_to')"
              >
                {{ t('workspace.storage.api_field.go_to') }}
              </button>
              <button
                type="button"
                class="tool-btn small"
                @click="changeLink(id)"
                :title="t('workspace.storage.api_field.change')"
              >
                {{ t('workspace.storage.api_field.change') }}
              </button>
              <button
                type="button"
                class="tool-btn small danger"
                @click="removeId(id)"
                :title="t('workspace.storage.api_field.unlink')"
              >
                ✕
              </button>
            </span>
          </header>

          <dl v-if="isExpanded(id) && (field.map ?? []).length" class="api-field-rows">
            <template v-for="(m, idx) in field.map ?? []" :key="m.key + ':' + idx">
              <dt>{{ rowLabel(idx, m.key) }}</dt>
              <dd>
                <template v-for="(s, _i) in [shapeFor(rows[id]?.[m.key], sourceFields[m.key])]" :key="m.key">
                  <span v-if="s.kind === 'tags'" class="api-cell-tags">
                    <span v-for="tag in s.items" :key="tag" class="tag-chip">{{ tag }}</span>
                    <span v-if="s.items.length === 0" class="muted small">-</span>
                  </span>
                  <ul v-else-if="s.kind === 'list'" class="api-cell-list">
                    <li v-for="(item, i) in s.items" :key="i">{{ display(item) }}</li>
                    <li v-if="s.items.length === 0" class="muted small">-</li>
                  </ul>
                  <table v-else-if="s.kind === 'table'" class="api-cell-table">
                    <thead v-if="s.headers">
                      <tr>
                        <th v-for="h in s.headers" :key="h">{{ h }}</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="(row, ri) in s.rows" :key="ri">
                        <td v-for="(cell, ci) in row" :key="ci">{{ display(cell) }}</td>
                      </tr>
                    </tbody>
                  </table>
                  <span v-else>{{ s.text }}</span>
                </template>
              </dd>
            </template>
          </dl>
        </section>
        </template>
      </draggable>

      <!-- Add / pick controls. Single replaces; multi appends. -->
      <div class="api-field-empty">
        <span v-if="!ids.length" class="muted small">
          {{ t('workspace.storage.api_field.empty') }}
        </span>
        <button
          type="button"
          class="tool-btn primary small"
          @click="openPicker"
        >
          {{ linkLabel }}
        </button>
        <button
          type="button"
          class="tool-btn small"
          :disabled="!(field.map ?? []).length"
          @click="createOpen = true"
        >
          {{ createLabel }}
        </button>
      </div>
    </template>

    <APIFieldPicker
      :open="pickerOpen"
      :source-template="collection"
      :filter-spec="field.filter ?? null"
      @close="closePicker"
      @pick="onPick"
    />
    <ReferenceCreateDialog
      :open="createOpen"
      :target-template="collection"
      :columns="field.map ?? []"
      @close="createOpen = false"
      @created="onCreated"
    />
  </div>
</template>
