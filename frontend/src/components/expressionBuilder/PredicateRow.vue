<script setup lang="ts">
/*
 * PredicateRow — renders one Predicate inside a Rule, with kind-
 * specific controls (boolean switch, enum value checkboxes, number
 * comparator, date helper). Mutates the predicate in place; parent
 * component owns the predicate array and hands a single entry down
 * here for editing.
 */
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { SwitchField } from "../fields";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import {
  DateOp,
  EnumOp,
  NumberOp,
  RuleKind,
  type DateOpDescriptor,
  type Operator,
  type Predicate,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/expression/builder";

const props = defineProps<{
  predicate: Predicate;
  field: Field | null;
  enumOps: Operator[];
  numberOps: Operator[];
  dateOps: DateOpDescriptor[];
}>();

const { t } = useI18n();

const fieldLabel = computed(() => props.field?.label || props.field?.key || props.predicate.fieldKey);

// Field options come off the template Field as an `any[]` shape;
// normalise to {value, label} pairs for the value checkboxes.
const fieldOptions = computed<Array<{ value: string; label: string }>>(() => {
  const raw = (props.field?.options ?? []) as any[];
  return raw.map((o) => ({
    value: String(o?.value ?? ""),
    label: String(o?.label ?? o?.value ?? ""),
  }));
});

const dateOpDescriptor = computed<DateOpDescriptor | null>(
  () => props.dateOps.find((d) => d.op === props.predicate.dateOp) ?? null,
);

// ── Mutators ────────────────────────────────────────────────────

function setBoolValue(v: boolean) {
  props.predicate.boolValue = v;
}

function setEnumOp(v: string) {
  props.predicate.enumOp = v as EnumOp;
}

function toggleEnumValue(value: string, on: boolean) {
  const cur = props.predicate.enumValues ?? [];
  if (on) {
    if (!cur.includes(value)) {
      props.predicate.enumValues = [...cur, value];
    }
  } else {
    props.predicate.enumValues = cur.filter((v) => v !== value);
  }
}

function setNumberOp(v: string) {
  props.predicate.numberOp = v as NumberOp;
}

function setNumberValue(v: number) {
  props.predicate.numberValue = isFinite(v) ? v : 0;
}

function setDateOp(v: string) {
  props.predicate.dateOp = v as DateOp;
  // Drop the arg when switching to a no-arg helper so stale values
  // don't leak into the compiled source.
  const desc = props.dateOps.find((d) => d.op === v);
  if (desc && !desc.hasArg) {
    props.predicate.dateArg = null;
  } else if (desc && desc.hasArg && props.predicate.dateArg == null) {
    props.predicate.dateArg = 0;
  }
}

function setDateArg(v: number) {
  props.predicate.dateArg = isFinite(v) ? v : 0;
}
</script>

<template>
  <div class="expr-pred-row">
    <span class="expr-pred-field">{{ fieldLabel }}</span>

    <!-- Boolean: just true/false toggle -->
    <SwitchField
      v-if="predicate.kind === RuleKind.KindBoolean"
      :model-value="predicate.boolValue ?? true"
      :on-label="t('common.on')"
      :off-label="t('common.off')"
      @update:model-value="setBoolValue"
    />

    <!-- Enum: op picker + value checkboxes from field options -->
    <template v-else-if="predicate.kind === RuleKind.KindEnum">
      <select
        class="expr-pred-op"
        :value="predicate.enumOp ?? ''"
        @change="setEnumOp(($event.target as HTMLSelectElement).value)"
      >
        <option v-for="op in enumOps" :key="op.op" :value="op.op">
          {{ t(op.labelKey) }}
        </option>
      </select>
      <div class="expr-pred-enum-values">
        <label
          v-for="opt in fieldOptions"
          :key="opt.value"
          class="expr-pred-enum-opt"
        >
          <input
            type="checkbox"
            :checked="(predicate.enumValues ?? []).includes(opt.value)"
            @change="toggleEnumValue(opt.value, ($event.target as HTMLInputElement).checked)"
          />
          {{ opt.label }}
        </label>
      </div>
    </template>

    <!-- Number: op picker + numeric input -->
    <template v-else-if="predicate.kind === RuleKind.KindNumber">
      <select
        class="expr-pred-op"
        :value="predicate.numberOp ?? ''"
        @change="setNumberOp(($event.target as HTMLSelectElement).value)"
      >
        <option v-for="op in numberOps" :key="op.op" :value="op.op">
          {{ t(op.labelKey) }}
        </option>
      </select>
      <input
        type="number"
        class="expr-pred-num"
        :value="predicate.numberValue ?? 0"
        @input="setNumberValue(Number(($event.target as HTMLInputElement).value))"
      />
    </template>

    <!-- Date: helper picker + optional days-arg input -->
    <template v-else-if="predicate.kind === RuleKind.KindDate">
      <select
        class="expr-pred-op"
        :value="predicate.dateOp ?? ''"
        @change="setDateOp(($event.target as HTMLSelectElement).value)"
      >
        <option v-for="op in dateOps" :key="op.op" :value="op.op">
          {{ t(op.labelKey) }}
        </option>
      </select>
      <input
        v-if="dateOpDescriptor?.hasArg"
        type="number"
        class="expr-pred-num"
        :value="predicate.dateArg ?? 0"
        @input="setDateArg(Number(($event.target as HTMLInputElement).value))"
      />
    </template>
  </div>
</template>
