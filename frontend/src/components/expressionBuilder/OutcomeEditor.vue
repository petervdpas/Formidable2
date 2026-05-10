<script setup lang="ts">
/*
 * OutcomeEditor — text source picker + color/bg/classes for one
 * Outcome. Used both for per-rule outcomes and for the dialog's
 * default outcome. Mutates the outcome in place.
 *
 * Color/bg/classes pickers use the generic Popup component: a
 * compact trigger (current swatch / chip list) opens a popup with
 * the full picker. Keeps the outcome row tight when collapsed and
 * expands only on demand.
 */
import { useI18n } from "vue-i18n";
import Popup from "../Popup.vue";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import {
  Outcome,
  TextKind,
  TextSource,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/expression/builder";

const props = defineProps<{
  outcome: Outcome;
  expressionFields: Field[];
  enumFields: Field[];
}>();

const { t } = useI18n();

// 11 distinct colors — 9 prime hues from styles/expression.css plus
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

// CSS utility classes the picker exposes. Color/bg utility classes
// (expr-text-* / expr-bg-*) are intentionally NOT here — the Color
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

// Background tint alpha — matches the .expr-bg-* declarations in
// styles/expression.css. Picking a swatch in the Background popup
// stores rgba(...) so the chip renders exactly as the matching
// expr-bg-* class would.
const BG_TINT_ALPHA = 0.18;

// White and black are NOT in the expr-bg-* tint set — they're
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

// ── Text source ─────────────────────────────────────────────────

function textKindOf(): string {
  return props.outcome.text?.kind ?? "";
}

function textValueOrFieldKey(): string {
  const ts = props.outcome.text;
  if (!ts) return "";
  return ts.kind === TextKind.TextKindLiteral ? (ts.value ?? "") : (ts.fieldKey ?? "");
}

function setTextKind(kind: string) {
  if (!kind) {
    props.outcome.text = undefined;
    return;
  }
  props.outcome.text = new TextSource({
    kind: kind as TextKind,
    value: "",
    fieldKey: "",
  });
}

function setTextValueOrFieldKey(v: string) {
  const ts = props.outcome.text;
  if (!ts) return;
  if (ts.kind === TextKind.TextKindLiteral) ts.value = v;
  else ts.fieldKey = v;
}

// ── Color / background ──────────────────────────────────────────

function setColor(v: string) { props.outcome.color = v || ""; }
function setBg(hex: string) {
  // Empty hex → clear; otherwise store the tinted rgba so the chip
  // renders identically to the matching expr-bg-* class.
  props.outcome.bg = hex ? hexToBgValue(hex) : "";
}

function isColor(hex: string): boolean {
  return (props.outcome.color ?? "") === hex;
}
function isBg(hex: string): boolean {
  return (props.outcome.bg ?? "") === hexToBgValue(hex);
}

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
    <!-- Text -->
    <label class="expr-outcome-row-label">
      {{ t('workspace.templates.expression_builder.outcome.text') }}
    </label>
    <div class="expr-outcome-row-control">
      <select
        class="expr-outcome-text-kind"
        :value="textKindOf()"
        @change="setTextKind(($event.target as HTMLSelectElement).value)"
      >
        <option value="">{{ t('workspace.templates.expression_builder.text_kind.none') }}</option>
        <option value="literal">{{ t('workspace.templates.expression_builder.text_kind.literal') }}</option>
        <option value="fieldValue">{{ t('workspace.templates.expression_builder.text_kind.field_value') }}</option>
        <option value="fieldLabel">{{ t('workspace.templates.expression_builder.text_kind.field_label') }}</option>
      </select>

      <input
        v-if="textKindOf() === 'literal'"
        type="text"
        class="expr-outcome-text-value"
        :value="textValueOrFieldKey()"
        @input="setTextValueOrFieldKey(($event.target as HTMLInputElement).value)"
      />
      <select
        v-else-if="textKindOf() === 'fieldValue'"
        class="expr-outcome-text-value"
        :value="textValueOrFieldKey()"
        @change="setTextValueOrFieldKey(($event.target as HTMLSelectElement).value)"
      >
        <option value="">—</option>
        <option v-for="f in expressionFields" :key="f.key" :value="f.key">
          {{ f.label || f.key }}
        </option>
      </select>
      <select
        v-else-if="textKindOf() === 'fieldLabel'"
        class="expr-outcome-text-value"
        :value="textValueOrFieldKey()"
        @change="setTextValueOrFieldKey(($event.target as HTMLSelectElement).value)"
      >
        <option value="">—</option>
        <option v-for="f in enumFields" :key="f.key" :value="f.key">
          {{ f.label || f.key }}
        </option>
      </select>
    </div>

    <!-- Color (popup with 3×3 swatch grid + clear) -->
    <label class="expr-outcome-row-label">
      {{ t('workspace.templates.expression_builder.outcome.color') }}
    </label>
    <div class="expr-outcome-row-control">
      <Popup placement="right">
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
        <div class="expr-swatch-grid">
          <button
            v-for="c in COLOR_PALETTE"
            :key="c.value"
            type="button"
            class="expr-swatch"
            :class="{ selected: isColor(c.value) }"
            :style="{ background: c.value }"
            :title="c.label"
            @click="setColor(c.value)"
          ></button>
          <button
            type="button"
            class="expr-swatch expr-swatch-clear"
            :title="t('workspace.templates.expression_builder.text_kind.none')"
            @click="setColor('')"
          >×</button>
        </div>
      </Popup>
    </div>

    <!-- Background (same shape as Color) -->
    <label class="expr-outcome-row-label">
      {{ t('workspace.templates.expression_builder.outcome.bg') }}
    </label>
    <div class="expr-outcome-row-control">
      <Popup placement="right">
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
        <div class="expr-swatch-grid">
          <button
            v-for="c in COLOR_PALETTE"
            :key="c.value"
            type="button"
            class="expr-swatch"
            :class="{ selected: isBg(c.value) }"
            :style="{ background: c.value }"
            :title="c.label"
            @click="setBg(c.value)"
          ></button>
          <button
            type="button"
            class="expr-swatch expr-swatch-clear"
            :title="t('workspace.templates.expression_builder.text_kind.none')"
            @click="setBg('')"
          >×</button>
        </div>
      </Popup>
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
