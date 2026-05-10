<script setup lang="ts">
/*
 * OutcomeEditor — text source picker + color/bg/classes for one
 * Outcome. Used both for per-rule outcomes and for the dialog's
 * default outcome. Mutates the outcome in place.
 */
import { computed } from "vue";
import { useI18n } from "vue-i18n";
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

const textKind = computed<string>(() => props.outcome.text?.kind ?? "");

const textValueOrFieldKey = computed<string>(() => {
  const ts = props.outcome.text;
  if (!ts) return "";
  return ts.kind === TextKind.TextKindLiteral ? (ts.value ?? "") : (ts.fieldKey ?? "");
});

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
  if (ts.kind === TextKind.TextKindLiteral) {
    ts.value = v;
  } else {
    ts.fieldKey = v;
  }
}

function setColor(v: string) {
  props.outcome.color = v || "";
}

function setBg(v: string) {
  props.outcome.bg = v || "";
}

function setClassesFromCSV(v: string) {
  const arr = v.split(/\s+/).filter(Boolean);
  props.outcome.classes = arr.length > 0 ? arr : undefined;
}

const classesCSV = computed<string>(() => (props.outcome.classes ?? []).join(" "));
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
        :value="textKind"
        @change="setTextKind(($event.target as HTMLSelectElement).value)"
      >
        <option value="">{{ t('workspace.templates.expression_builder.text_kind.none') }}</option>
        <option value="literal">{{ t('workspace.templates.expression_builder.text_kind.literal') }}</option>
        <option value="fieldValue">{{ t('workspace.templates.expression_builder.text_kind.field_value') }}</option>
        <option value="fieldLabel">{{ t('workspace.templates.expression_builder.text_kind.field_label') }}</option>
      </select>

      <input
        v-if="textKind === 'literal'"
        type="text"
        class="expr-outcome-text-value"
        :value="textValueOrFieldKey"
        @input="setTextValueOrFieldKey(($event.target as HTMLInputElement).value)"
      />
      <select
        v-else-if="textKind === 'fieldValue'"
        class="expr-outcome-text-value"
        :value="textValueOrFieldKey"
        @change="setTextValueOrFieldKey(($event.target as HTMLSelectElement).value)"
      >
        <option value="">—</option>
        <option v-for="f in expressionFields" :key="f.key" :value="f.key">
          {{ f.label || f.key }}
        </option>
      </select>
      <select
        v-else-if="textKind === 'fieldLabel'"
        class="expr-outcome-text-value"
        :value="textValueOrFieldKey"
        @change="setTextValueOrFieldKey(($event.target as HTMLSelectElement).value)"
      >
        <option value="">—</option>
        <option v-for="f in enumFields" :key="f.key" :value="f.key">
          {{ f.label || f.key }}
        </option>
      </select>
    </div>

    <!-- Color -->
    <label class="expr-outcome-row-label">
      {{ t('workspace.templates.expression_builder.outcome.color') }}
    </label>
    <div class="expr-outcome-row-control">
      <input
        type="text"
        class="expr-outcome-color"
        :value="outcome.color ?? ''"
        @input="setColor(($event.target as HTMLInputElement).value)"
        placeholder="#c0392b"
      />
    </div>

    <!-- Background -->
    <label class="expr-outcome-row-label">
      {{ t('workspace.templates.expression_builder.outcome.bg') }}
    </label>
    <div class="expr-outcome-row-control">
      <input
        type="text"
        class="expr-outcome-color"
        :value="outcome.bg ?? ''"
        @input="setBg(($event.target as HTMLInputElement).value)"
        placeholder="#fff"
      />
    </div>

    <!-- Classes -->
    <label class="expr-outcome-row-label">
      {{ t('workspace.templates.expression_builder.outcome.classes') }}
    </label>
    <div class="expr-outcome-row-control">
      <input
        type="text"
        class="expr-outcome-classes"
        :value="classesCSV"
        @input="setClassesFromCSV(($event.target as HTMLInputElement).value)"
        :placeholder="t('workspace.templates.expression_builder.outcome.classes_hint')"
      />
    </div>
  </div>
</template>
