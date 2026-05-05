<script setup lang="ts">
import { computed } from "vue";
import draggable from "vuedraggable";
import { TextField, SelectField } from "../fields";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

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
// Drag-reorder + paste-data + popup-style dropdown defer to a
// follow-up; v1 uses a native <select>.

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
      handle=".list-row-handle"
      :animation="150"
      ghost-class="list-row-ghost"
      chosen-class="list-row-chosen"
      drag-class="list-row-drag"
      :item-key="(_e: string, i: number) => i"
    >
      <template #item="{ index: i, element: item }">
        <div class="list-row">
          <span class="list-row-handle" :title="'Drag to reorder'" aria-hidden="true">⠿</span>

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
            class="list-btn remove"
            @click="remove(i)"
            aria-label="Remove item"
          >−</button>
        </div>
      </template>
    </draggable>

    <button
      v-if="!field.readonly"
      type="button"
      class="list-btn add"
      @click="add('')"
    >+</button>
  </div>
</template>

<style scoped>
.list-field {
    display: flex;
    flex-direction: column;
    gap: 6px;
}
.list-rows {
    display: flex;
    flex-direction: column;
    gap: 6px;
}
.list-row {
    display: flex;
    gap: 6px;
    align-items: center;
}
.list-row-handle {
    cursor: grab;
    user-select: none;
    font-size: 16px;
    line-height: 1;
    opacity: 0.7;
    padding: 0 2px;
    flex: 0 0 auto;
}
.list-row-handle:active { cursor: grabbing; }

.list-row > :deep(.field-input),
.list-row > :deep(.select-field-control),
.list-row > :deep(select) {
    flex: 1 1 auto;
}
.list-btn {
    flex: 0 0 auto;
    width: 32px;
    height: 34px;
    appearance: none;
    border: 1px solid var(--color-border);
    background: var(--color-bg);
    color: var(--color-text);
    border-radius: var(--radius-md);
    cursor: pointer;
    font-size: 16px;
    line-height: 1;
    font-weight: 600;
}
.list-btn:hover { background: var(--color-surface-2); }
.list-btn.add { width: 100%; }

/* Sortable.js visual states (vuedraggable forwards class names). */
.list-row-ghost {
    opacity: 0.35;
    filter: saturate(0.4);
}
.list-row-chosen {
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.35);
}
.list-row-drag { cursor: grabbing; }

/* Fixed-only mode: highlight rows whose value isn't in the allowed
   set (mirrors the original's "⚠ Not in list" affordance). */
.list-row :deep(.invalid),
.list-row :deep(select.invalid) {
    border-color: var(--color-danger, #dc2626);
}
</style>
