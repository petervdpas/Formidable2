<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import draggable from "vuedraggable";
import { TextField, SelectField } from "../fields";
import PasteDataDialog from "../PasteDataDialog.vue";
import { useConfig } from "../../composables/useConfig";
import { rowsToListValues } from "../../utils/pasteData";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const { t } = useI18n();
const { config } = useConfig();
const showPaste = computed(() => !!config.value?.show_paste_buttons);
const pasteOpen = ref(false);

// Local narrow shape — `SelectField`'s SelectOption union also allows
// plain strings; we always build the object form here.
type ListOption = { value: string; label: string };

// DnD scope — unique per component instance. The same list field
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

// ── Items (always a string[]) ────────────────────────────────────────
const items = computed<string[]>({
  get: () => {
    const v = props.modelValue;
    if (Array.isArray(v)) return v.map(String);
    return [];
  },
  set: (v) => emit("update:modelValue", v),
});

function setItem(i: number, value: string) {
  const next = items.value.slice();
  next[i] = value;
  items.value = next;
}

function add(initial = "") {
  items.value = [...items.value, initial];
}

function remove(i: number) {
  items.value = items.value.filter((_, j) => j !== i);
}

function onPasteProcess(rows: string[][]) {
  const values = rowsToListValues(rows);
  if (values.length > 0) {
    items.value = [...items.value, ...values];
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
// fixed set means the user picked "Custom…" — render a text input.
function isCustom(row: string): boolean {
  if (mode.value !== "dropdown") return false;
  return !fixedValues.value.has(row);
}

// Dropdown options assembled per render — fixed entries plus the
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

// In fixed-only mode, a stored value not in the set is "invalid" —
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
      v-model="items"
      tag="div"
      class="list-rows"
      :group="dndScope"
      handle=".dnd-handle"
      :animation="150"
      ghost-class="dnd-ghost"
      chosen-class="dnd-chosen"
      drag-class="dnd-drag"
      :item-key="(_e: string, i: number) => i"
    >
      <template #item="{ index: i, element: item }">
        <div class="list-row">
          <span class="dnd-handle" :title="t('workspace.storage.field.drag_to_reorder')" aria-hidden="true">⠿</span>

          <!-- Free text (no options OR custom-only) -->
          <TextField
            v-if="mode === 'free'"
            :model-value="item"
            @update:model-value="(v) => setItem(i, v)"
            :placeholder="customPlaceholder"
            :readonly="field.readonly"
          />

          <!-- Mixed: dropdown unless this row holds a value not in the
               fixed set (user picked Custom or pasted something custom). -->
          <template v-else-if="mode === 'dropdown'">
            <TextField
              v-if="isCustom(item)"
              :model-value="item"
              @update:model-value="(v) => setItem(i, v)"
              :placeholder="customPlaceholder"
              :readonly="field.readonly"
            />
            <SelectField
              v-else
              :model-value="item"
              @update:model-value="(v) => onSelect(i, v)"
              :options="selectOptions"
            />
          </template>

          <!-- Fixed-only — locked to the allowed set. -->
          <SelectField
            v-else
            :model-value="item"
            @update:model-value="(v) => setItem(i, v)"
            :options="selectOptions"
            :class="{ invalid: isInvalid(item) }"
          />

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
