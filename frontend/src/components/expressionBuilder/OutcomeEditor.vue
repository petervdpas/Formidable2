<script setup lang="ts">
/*
 * OutcomeEditor - text source picker + color/bg/classes for one
 * Outcome. Used both for per-rule outcomes and for the dialog's
 * default outcome. Mutates the outcome in place.
 *
 * Color/bg/classes pickers use the generic Popup component: a
 * compact trigger (current swatch / chip list) opens a popup with
 * the full picker. Keeps the outcome row tight when collapsed and
 * expands only on demand.
 */
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import draggable from "vuedraggable";
import Popup from "../Popup.vue";
import SwatchPicker, { type SwatchOption } from "../SwatchPicker.vue";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import {
  Outcome,
  TextKind,
  TextSource,
  type TextSourceOption,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/expression/builder";

const props = defineProps<{
  outcome: Outcome;
  /** Backend-assembled field-value sources (displayable fields + formulas). */
  textSources: TextSourceOption[];
  enumFields: Field[];
}>();

// Split the backend list into Fields and Formulas for grouped <optgroup>s.
const fieldSources = computed(() => props.textSources.filter((s) => s.group === "field"));
const formulaSources = computed(() => props.textSources.filter((s) => s.group === "formula"));

// Mirrors builder.MaxConcatParts. Wails doesn't expose Go consts so
// we pin the value here; the backend re-checks at Compile time, so a
// stale frontend can never sneak past the cap.
const MAX_PARTS = 10;

const { t } = useI18n();

// 11 distinct colors - 9 prime hues from styles/expression.css plus
// black and white. Hex-coded so the compile output references the
// value directly. Laid out in a 3×4 grid with the clear (×) button
// occupying the 12th cell (see template).
const COLOR_PALETTE: Array<{ value: string; label: string }> = [
  { value: "#e84e4e", label: "red" },
  { value: "#ff9438", label: "orange" },
  { value: "#f5dd5d", label: "yellow" },
  { value: "#50c878", label: "green" },
  { value: "#2bb1a6", label: "teal" },
  { value: "#4a90e2", label: "blue" },
  { value: "#a064dc", label: "purple" },
  { value: "#e472b8", label: "pink" },
  { value: "#8a93a6", label: "gray" },
  { value: "#000000", label: "black" },
  { value: "#ffffff", label: "white" },
];

// SwatchPicker option list mirroring COLOR_PALETTE - `color` field
// drives inline-style background on each cell.
const COLOR_PALETTE_OPTIONS: SwatchOption[] = COLOR_PALETTE.map((c) => ({
  value: c.value,
  label: c.label,
  color: c.value,
}));

// CSS utility classes the picker exposes. Color/bg utility classes
// (expr-text-* / expr-bg-*) are intentionally NOT here - the Color
// and Background pickers cover the same ground (Color emits the hex,
// Background emits the tinted rgba 0.18 form expr-bg-* uses). The
// classes in styles/expression.css remain for backward compat with
// hand-authored templates.
const EXPRESSION_CLASSES = [
  "expr-bold",
  "expr-italic",
  "expr-blink",
  "expr-pulse",
  "expr-scrolling",
  "expr-error",
];

// Background tint alpha - matches the .expr-bg-* declarations in
// styles/expression.css. Picking a swatch in the Background popup
// stores rgba(...) so the chip renders exactly as the matching
// expr-bg-* class would.
const BG_TINT_ALPHA = 0.18;

// White and black are NOT in the expr-bg-* tint set - they're
// neutrals that the user means literally. Tinting white/black at
// 18% alpha on a dark theme makes them effectively invisible, so
// we keep them solid.
const NEUTRALS = new Set(["#ffffff", "#000000"]);

function hexToBgValue(hex: string): string {
  if (NEUTRALS.has(hex.toLowerCase())) return hex;
  const r = parseInt(hex.slice(1, 3), 16);
  const g = parseInt(hex.slice(3, 5), 16);
  const b = parseInt(hex.slice(5, 7), 16);
  return `rgba(${r}, ${g}, ${b}, ${BG_TINT_ALPHA})`;
}

// ── Text parts ──────────────────────────────────────────────────
//
// Outcome's chip text is an ordered list of parts joined with `+`.
// Each part is one of: literal | field value | option label. The
// editor migrates the legacy single `outcome.text` field into
// `outcome.parts` lazily on first edit so existing single-source
// configs keep working without an explicit upgrade step.

function ensureParts(): TextSource[] {
  if (props.outcome.parts && props.outcome.parts.length > 0) {
    return props.outcome.parts;
  }
  if (props.outcome.text) {
    props.outcome.parts = [props.outcome.text];
  } else {
    props.outcome.parts = [];
  }
  props.outcome.text = undefined;
  return props.outcome.parts;
}

// `parts` is a computed view: returns the parts array if present,
// or wraps the legacy `text` field in a one-element list so the
// template renders pre-migration outcomes the same as post.
const parts = computed<TextSource[]>(() => {
  if (props.outcome.parts && props.outcome.parts.length > 0) return props.outcome.parts;
  if (props.outcome.text) return [props.outcome.text];
  return [];
});

// Vuedraggable v-model needs a writable computed. Reorder writes
// the new ordering into outcome.parts and clears the legacy text.
const partsModel = computed<TextSource[]>({
  get: () => parts.value,
  set: (v) => {
    props.outcome.parts = v.length > 0 ? v : undefined;
    props.outcome.text = undefined;
  },
});

function addPart() {
  if (parts.value.length >= MAX_PARTS) return;
  const list = ensureParts();
  list.push(new TextSource({
    kind: TextKind.TextKindLiteral,
    value: "",
    fieldKey: "",
  }));
}

function removePart(i: number) {
  const list = ensureParts();
  list.splice(i, 1);
  if (list.length === 0) {
    props.outcome.parts = undefined;
  }
}

function setPartKind(i: number, kind: string) {
  const list = ensureParts();
  if (!kind) {
    removePart(i);
    return;
  }
  list[i] = new TextSource({
    kind: kind as TextKind,
    value: "",
    fieldKey: "",
  });
}

function setPartValue(i: number, v: string) {
  const list = ensureParts();
  const ts = list[i];
  if (!ts) return;
  if (ts.kind === TextKind.TextKindLiteral) ts.value = v;
  else ts.fieldKey = v;
}

function partValueOrKey(ts: TextSource): string {
  return ts.kind === TextKind.TextKindLiteral ? (ts.value ?? "") : (ts.fieldKey ?? "");
}

// ── Color / background ──────────────────────────────────────────

function setColor(v: string) { props.outcome.color = v || ""; }
function setBg(hex: string) {
  // Empty hex → clear; otherwise store the tinted rgba so the chip
  // renders identically to the matching expr-bg-* class.
  props.outcome.bg = hex ? hexToBgValue(hex) : "";
}

// Reverse-lookup for the bg picker: outcome.bg stores the tinted
// rgba(...), but the SwatchPicker matches by hex. This computed maps
// the stored tinted value back to the matching palette hex so the
// picker can highlight the active swatch.
const bgHex = computed<string>(() => {
  const v = props.outcome.bg ?? "";
  if (!v) return "";
  const found = COLOR_PALETTE.find((c) => hexToBgValue(c.value) === v);
  return found?.value ?? "";
});

// ── Classes ─────────────────────────────────────────────────────

function isClassSelected(name: string): boolean {
  return (props.outcome.classes ?? []).includes(name);
}

function toggleClass(name: string) {
  const cur = props.outcome.classes ?? [];
  const next = cur.includes(name) ? cur.filter((c) => c !== name) : [...cur, name];
  props.outcome.classes = next.length > 0 ? next : undefined;
}

function classDisplayName(name: string): string {
  return name.replace(/^expr-/, "");
}
</script>

<template>
  <div class="expr-outcome-grid">
    <!-- Text parts -->
    <label class="expr-outcome-row-label top">
      {{ t('workspace.templates.expression_builder.outcome.text') }}
    </label>
    <div class="expr-outcome-row-control expr-outcome-text-parts">
      <button
        class="tool-btn expr-text-part-add"
        type="button"
        :disabled="parts.length >= MAX_PARTS"
        @click="addPart"
      >
        {{ t('workspace.templates.expression_builder.outcome.text_part_add') }}
      </button>
      <draggable
        v-if="parts.length"
        v-model="partsModel"
        tag="ul"
        class="expr-text-part-list"
        handle=".dnd-handle"
        :animation="150"
        ghost-class="dnd-ghost"
        chosen-class="dnd-chosen"
        drag-class="dnd-drag"
        :item-key="(_e: TextSource, i: number) => i"
      >
        <template #item="{ index: i, element: p }">
          <li class="expr-text-part-row">
            <span
              class="dnd-handle"
              :title="t('workspace.templates.expression_builder.outcome.text_part_reorder')"
              aria-hidden="true"
            >⠿</span>
            <select
              class="expr-outcome-text-kind"
              :value="p.kind"
              @change="setPartKind(i, ($event.target as HTMLSelectElement).value)"
            >
              <option value="literal">{{ t('workspace.templates.expression_builder.text_kind.literal') }}</option>
              <option value="fieldValue">{{ t('workspace.templates.expression_builder.text_kind.field_value') }}</option>
              <option value="fieldLabel">{{ t('workspace.templates.expression_builder.text_kind.field_label') }}</option>
            </select>
            <input
              v-if="p.kind === TextKind.TextKindLiteral"
              type="text"
              class="expr-outcome-text-value"
              :value="partValueOrKey(p)"
              @input="setPartValue(i, ($event.target as HTMLInputElement).value)"
            />
            <select
              v-else-if="p.kind === TextKind.TextKindFieldValue"
              class="expr-outcome-text-value"
              :value="partValueOrKey(p)"
              @change="setPartValue(i, ($event.target as HTMLSelectElement).value)"
            >
              <option value="">-</option>
              <optgroup
                v-if="fieldSources.length"
                :label="t('workspace.templates.expression_builder.text_source_group.field')"
              >
                <option v-for="s in fieldSources" :key="s.key" :value="s.key">{{ s.label }}</option>
              </optgroup>
              <optgroup
                v-if="formulaSources.length"
                :label="t('workspace.templates.expression_builder.text_source_group.formula')"
              >
                <option v-for="s in formulaSources" :key="s.key" :value="s.key">{{ s.label }}</option>
              </optgroup>
            </select>
            <select
              v-else-if="p.kind === TextKind.TextKindFieldLabel"
              class="expr-outcome-text-value"
              :value="partValueOrKey(p)"
              @change="setPartValue(i, ($event.target as HTMLSelectElement).value)"
            >
              <option value="">-</option>
              <option v-for="f in enumFields" :key="f.key" :value="f.key">
                {{ f.label || f.key }}
              </option>
            </select>
            <button
              class="expr-builder-rule-remove"
              type="button"
              :title="t('workspace.templates.expression_builder.outcome.text_part_remove')"
              @click="removePart(i)"
            >×</button>
          </li>
        </template>
      </draggable>
    </div>

    <!-- Color (popup with 3-col swatch grid + clear) -->
    <label class="expr-outcome-row-label">
      {{ t('workspace.templates.expression_builder.outcome.color') }}
    </label>
    <div class="expr-outcome-row-control">
      <SwatchPicker
        :model-value="outcome.color ?? ''"
        :options="COLOR_PALETTE_OPTIONS"
        placement="right"
        :cols="3"
        size="1.8rem"
        clearable
        :clear-title="t('workspace.templates.expression_builder.text_kind.none')"
        @update:model-value="setColor"
      >
        <template #trigger="{ toggle, open }">
          <button
            type="button"
            class="expr-color-trigger"
            :class="{ open }"
            :style="outcome.color ? { background: outcome.color } : undefined"
            @click="toggle"
          >
            <span v-if="!outcome.color" class="muted small">{{ t('workspace.templates.expression_builder.text_kind.none') }}</span>
          </button>
        </template>
      </SwatchPicker>
    </div>

    <!-- Background (same shape as Color, value transformed to tinted rgba on save) -->
    <label class="expr-outcome-row-label">
      {{ t('workspace.templates.expression_builder.outcome.bg') }}
    </label>
    <div class="expr-outcome-row-control">
      <SwatchPicker
        :model-value="bgHex"
        :options="COLOR_PALETTE_OPTIONS"
        placement="right"
        :cols="3"
        size="1.8rem"
        clearable
        :clear-title="t('workspace.templates.expression_builder.text_kind.none')"
        @update:model-value="setBg"
      >
        <template #trigger="{ toggle, open }">
          <button
            type="button"
            class="expr-color-trigger"
            :class="{ open }"
            :style="outcome.bg ? { background: outcome.bg } : undefined"
            @click="toggle"
          >
            <span v-if="!outcome.bg" class="muted small">{{ t('workspace.templates.expression_builder.text_kind.none') }}</span>
          </button>
        </template>
      </SwatchPicker>
    </div>

    <!-- Classes (popup with checkbox grid) -->
    <label class="expr-outcome-row-label">
      {{ t('workspace.templates.expression_builder.outcome.classes') }}
    </label>
    <div class="expr-outcome-row-control">
      <Popup placement="above">
        <template #trigger="{ toggle, open }">
          <button
            type="button"
            class="expr-classes-trigger"
            :class="{ open }"
            @click="toggle"
          >
            <template v-if="(outcome.classes ?? []).length">
              <span
                v-for="c in outcome.classes"
                :key="c"
                class="expr-classes-trigger-chip"
                :class="c"
              >{{ classDisplayName(c) }}</span>
            </template>
            <template v-else>
              <span class="muted small">{{ t('workspace.templates.expression_builder.outcome.classes_hint') }}</span>
            </template>
          </button>
        </template>
        <div class="expr-classes-grid">
          <label
            v-for="c in EXPRESSION_CLASSES"
            :key="c"
            class="expr-classes-item"
          >
            <input
              type="checkbox"
              :checked="isClassSelected(c)"
              @change="toggleClass(c)"
            />
            <span :class="c">{{ classDisplayName(c) }}</span>
          </label>
        </div>
      </Popup>
    </div>
  </div>
</template>
