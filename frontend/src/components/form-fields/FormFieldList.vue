<script setup lang="ts">
import { computed, inject, ref } from "vue";
import { useI18n } from "vue-i18n";
import draggable from "vuedraggable";
import { TextField, SelectField } from "../fields";
import PasteDataDialog from "../PasteDataDialog.vue";
import { useConfig } from "../../composables/useConfig";
import { rowsToListValues } from "../../utils/pasteData";
import { FORM_FIELD_OPS_KEY } from "../../composables/formFieldOps";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const { t } = useI18n();
const { config } = useConfig();
const showPaste = computed(() => !!config.value?.show_paste_buttons);
const pasteOpen = ref(false);

// List sort + dedup run on the backend (it fetches the field from the
// saved record, sorts/dedups, returns the value); we apply the result
// and the normal Save persists it. Hidden when no ops context (e.g. the
// plugin run dialog renders fields in isolation).
const fieldOps = inject(FORM_FIELD_OPS_KEY, null);
const showSort = computed(() => !!config.value?.show_sort_buttons && !!fieldOps);
const showDedup = computed(() => !!config.value?.show_dedup_buttons && !!fieldOps);
const sortDir = ref<"asc" | "desc">("asc");

async function doSort() {
  const next = await fieldOps?.sortField(props.field.key, { direction: sortDir.value });
  if (next !== undefined) {
    rows.value = (next as unknown[]).map(parseRow);
    sortDir.value = sortDir.value === "asc" ? "desc" : "asc";
  }
}

async function doDedup() {
  const next = await fieldOps?.dedupField(props.field.key);
  if (next !== undefined) rows.value = (next as unknown[]).map(parseRow);
}

// Local narrow shape - `SelectField`'s SelectOption union also allows
// plain strings; we always build the object form here.
type ListOption = { value: string; label: string };

// DnD scope - unique per component instance. The same list field
// rendered inside multiple loop entries needs distinct scopes so
// vuedraggable doesn't accept items dragged across instances.
// Mirrors the original's `list:<key>:<loopChain>:<uuid>` scheme.
const dndScope =
  "list:" +
  (typeof crypto !== "undefined" && crypto.randomUUID
    ? crypto.randomUUID()
    : Math.random().toString(36).slice(2)
  ).slice(0, 8);

// FormFieldList mirrors the original Formidable list field
// (utils/listItemFactory.js):
//
//   no options             → free-text rows
//   only fixed options     → dropdown rows, value locked to allowed set
//   only `[[custom]]`      → free-text rows with placeholder from its label
//   mixed fixed + custom   → dropdown with fixed options + a "Custom…"
//                            entry; picking it flips that row to free text
//
// Storage shape stays a flat string[] regardless of mode.
//
// Popup-style dropdown defers to a follow-up; v1 uses a native <select>.

const CUSTOM_MARKER = "[[custom]]";

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

// ── Rows: text + indent per item ─────────────────────────────────────
// Storage stays a flat array. A row is a plain string (indent 0, unchanged) or a
// {text, indent} object (an indented row). Indent 0 serialises back to a plain
// string, so a list you never indent stays a pure string[] on disk and nothing
// downstream breaks.
type Row = { text: string; indent: number };
const MAX_INDENT = 6;

function parseRow(v: unknown): Row {
  if (v && typeof v === "object" && !Array.isArray(v)) {
    const o = v as Record<string, unknown>;
    const raw = typeof o.indent === "number" ? o.indent : 0;
    return {
      text: String(o.text ?? o.value ?? ""),
      indent: Math.max(0, Math.min(MAX_INDENT, Math.round(raw))),
    };
  }
  return { text: String(v ?? ""), indent: 0 };
}

function serializeRow(r: Row): unknown {
  return r.indent > 0 ? { text: r.text, indent: r.indent } : r.text;
}

// Invariant: the first row is flush (indent 0) and a row can be at most one level
// deeper than the row above it. Enforced on every write, so an orphan indent
// (first row indented, or a jumped level) is impossible, not just discouraged.
function normalizeIndents(rs: Row[]): Row[] {
  let prev = -1;
  return rs.map((r, i) => {
    const max = Math.min(MAX_INDENT, i === 0 ? 0 : prev + 1);
    const indent = Math.max(0, Math.min(r.indent, max));
    prev = indent;
    return r.indent === indent ? r : { ...r, indent };
  });
}

// The deepest indent this row may take, given the row above it.
function maxIndentFor(i: number): number {
  return Math.min(MAX_INDENT, i === 0 ? 0 : rows.value[i - 1].indent + 1);
}

const rows = computed<Row[]>({
  get: () => (Array.isArray(props.modelValue) ? props.modelValue.map(parseRow) : []),
  set: (rs) => emit("update:modelValue", normalizeIndents(rs).map(serializeRow)),
});

function setItem(i: number, text: string) {
  const next = rows.value.slice();
  next[i] = { ...next[i], text };
  rows.value = next;
}

// Indent/outdent one row, clamped to [0, MAX_INDENT]. Default 0 = flush left.
function setIndent(i: number, delta: number) {
  const next = rows.value.slice();
  const indent = Math.max(0, Math.min(MAX_INDENT, next[i].indent + delta));
  next[i] = { ...next[i], indent };
  rows.value = next;
}

function add(initial = "") {
  rows.value = [...rows.value, { text: initial, indent: 0 }];
}

function remove(i: number) {
  rows.value = rows.value.filter((_, j) => j !== i);
}

function onPasteProcess(pasted: string[][]) {
  const values = rowsToListValues(pasted);
  if (values.length > 0) {
    rows.value = [...rows.value, ...values.map((v) => ({ text: v, indent: 0 }))];
  }
  pasteOpen.value = false;
}

// ── Options-driven mode ──────────────────────────────────────────────
type Parsed = { fixed: ListOption[]; customLabel: string | null };

const parsed = computed<Parsed>(() => {
  const fixed: ListOption[] = [];
  let customLabel: string | null = null;
  for (const opt of props.field.options ?? []) {
    if (typeof opt === "string") {
      fixed.push({ value: opt, label: opt });
      continue;
    }
    if (opt && typeof opt === "object") {
      const o = opt as Record<string, unknown>;
      const value = String(o.value ?? "");
      const label = String(o.label ?? o.value ?? "");
      if (value === CUSTOM_MARKER) {
        customLabel = label || "Custom…";
      } else {
        fixed.push({ value, label });
      }
    }
  }
  return { fixed, customLabel };
});

const fixedValues = computed<Set<string>>(
  () => new Set(parsed.value.fixed.map((o) => o.value)),
);

type Mode = "free" | "dropdown" | "fixed-only";
const mode = computed<Mode>(() => {
  const { fixed, customLabel } = parsed.value;
  if (fixed.length === 0) return "free"; // no options OR custom-only
  if (customLabel) return "dropdown";    // fixed + custom marker
  return "fixed-only";                   // fixed only
});

const customPlaceholder = computed(() => parsed.value.customLabel ?? "");

// Per-row resolution: in dropdown mode, a value that's not in the
// fixed set means the user picked "Custom…" - render a text input.
function isCustom(row: string): boolean {
  if (mode.value !== "dropdown") return false;
  return !fixedValues.value.has(row);
}

// Dropdown options assembled per render - fixed entries plus the
// "Custom…" sentinel when the list allows custom values.
const selectOptions = computed<ListOption[]>(() => {
  const opts = parsed.value.fixed.slice();
  if (parsed.value.customLabel) {
    opts.push({ value: CUSTOM_MARKER, label: parsed.value.customLabel });
  }
  return opts;
});

// User picks "Custom…" → clear the row so the text input takes over;
// any other selection is the value itself.
function onSelect(i: number, picked: string) {
  if (picked === CUSTOM_MARKER) {
    setItem(i, "");
  } else {
    setItem(i, picked);
  }
}

// In fixed-only mode, a stored value not in the set is "invalid" -
// flag visually so the user knows it needs attention.
function isInvalid(row: string): boolean {
  if (mode.value !== "fixed-only") return false;
  if (row === "") return false; // empty = unfilled, not invalid
  return !fixedValues.value.has(row);
}
</script>

<template>
  <div class="list-field" :data-dnd-scope="dndScope">
    <draggable
      v-model="rows"
      tag="div"
      class="list-rows"
      :group="dndScope"
      handle=".dnd-handle"
      :animation="150"
      ghost-class="dnd-ghost"
      chosen-class="dnd-chosen"
      drag-class="dnd-drag"
      :item-key="(_e: Row, i: number) => i"
    >
      <template #item="{ index: i, element: item }">
        <div class="list-row" :style="{ marginInlineStart: item.indent * 1.5 + 'rem' }">
          <span class="dnd-handle" :title="t('workspace.storage.field.drag_to_reorder')" aria-hidden="true">⠿</span>

          <!-- Free text (no options OR custom-only) -->
          <TextField
            v-if="mode === 'free'"
            :model-value="item.text"
            @update:model-value="(v) => setItem(i, v)"
            :placeholder="customPlaceholder"
            :readonly="field.readonly"
          />

          <!-- Mixed: dropdown unless this row holds a value not in the
               fixed set (user picked Custom or pasted something custom). -->
          <template v-else-if="mode === 'dropdown'">
            <TextField
              v-if="isCustom(item.text)"
              :model-value="item.text"
              @update:model-value="(v) => setItem(i, v)"
              :placeholder="customPlaceholder"
              :readonly="field.readonly"
            />
            <SelectField
              v-else
              :model-value="item.text"
              @update:model-value="(v) => onSelect(i, v)"
              :options="selectOptions"
            />
          </template>

          <!-- Fixed-only - locked to the allowed set. -->
          <SelectField
            v-else
            :model-value="item.text"
            @update:model-value="(v) => setItem(i, v)"
            :options="selectOptions"
            :class="{ invalid: isInvalid(item.text) }"
          />

          <button
            v-if="!field.readonly"
            type="button"
            class="btn-ghost-icon"
            :disabled="item.indent === 0"
            :title="t('workspace.storage.field.outdent')"
            @click="setIndent(i, -1)"
          ><i class="fa-solid fa-outdent" aria-hidden="true"></i></button>
          <button
            v-if="!field.readonly"
            type="button"
            class="btn-ghost-icon"
            :disabled="item.indent >= maxIndentFor(i)"
            :title="t('workspace.storage.field.indent')"
            @click="setIndent(i, 1)"
          ><i class="fa-solid fa-indent" aria-hidden="true"></i></button>

          <button
            v-if="!field.readonly"
            type="button"
            class="btn-ghost-icon"
            @click="remove(i)"
            :aria-label="t('workspace.storage.field.remove_item')"
          >−</button>
        </div>
      </template>
    </draggable>

    <div v-if="!field.readonly" class="list-actions">
      <button
        type="button"
        class="btn-ghost-icon"
        :aria-label="t('workspace.storage.field.add_item')"
        :title="t('workspace.storage.field.add_item')"
        @click="add('')"
      >+</button>
      <button
        v-if="showPaste"
        type="button"
        class="btn-ghost-icon"
        :aria-label="t('paste.tooltip')"
        :title="t('paste.tooltip')"
        @click="pasteOpen = true"
      ><i class="fa-solid fa-paste"></i></button>
      <button
        v-if="showSort && rows.length > 1"
        type="button"
        class="btn-ghost-icon"
        :aria-label="t('workspace.storage.field.sort')"
        :title="t('workspace.storage.field.sort')"
        @click="doSort"
      ><i :class="sortDir === 'asc' ? 'fa-solid fa-arrow-down-a-z' : 'fa-solid fa-arrow-up-a-z'"></i></button>
      <button
        v-if="showDedup && rows.length > 1"
        type="button"
        class="btn-ghost-icon"
        :aria-label="t('workspace.storage.field.dedup')"
        :title="t('workspace.storage.field.dedup')"
        @click="doDedup"
      ><i class="fa-solid fa-broom"></i></button>
    </div>

    <PasteDataDialog
      :open="pasteOpen"
      :title="t('paste.list.title')"
      :subtitle="t('paste.list.subtitle')"
      @process="onPasteProcess"
      @cancel="pasteOpen = false"
    />
  </div>
</template>
