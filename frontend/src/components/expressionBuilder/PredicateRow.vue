<script setup lang="ts">
/*
 * PredicateRow - renders one Predicate inside a Rule, with kind-
 * specific controls (boolean switch, enum value checkboxes, number
 * comparator, date helper). Mutates the predicate in place; parent
 * component owns the predicate array and hands a single entry down
 * here for editing.
 */
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { SelectField, SwitchField } from "../fields";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import {
  DateOp,
  EnumOp,
  NumberOp,
  RuleKind,
  type DateOpDescriptor,
  type FieldOption,
  type Operator,
  type Predicate,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/expression/builder";

const props = defineProps<{
  predicate: Predicate;
  field: Field | null;
  /** Pre-resolved option list for the field (backend-driven via
   *  ExpressionSvc.BuilderFieldOptions in the parent modal). Empty
   *  for non-enumerable types and for facet fields whose binding
   *  doesn't resolve. */
  options: FieldOption[];
  enumOps: Operator[];
  numberOps: Operator[];
  dateOps: DateOpDescriptor[];
}>();

const { t } = useI18n();

const fieldLabel = computed(() => props.field?.label || props.field?.key || props.predicate.fieldKey);

const fieldOptions = computed<FieldOption[]>(() => props.options ?? []);

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

const selectedEnumValues = computed<string[]>(() => props.predicate.enumValues ?? []);

// Options the user can still add to the predicate (current options
// minus anything already chosen). Drives the add-value dropdown so
// the picker only offers fresh values.
const addableEnumOptions = computed<FieldOption[]>(() =>
  fieldOptions.value.filter((o) => !selectedEnumValues.value.includes(o.value)),
);

function labelForEnumValue(value: string): string {
  return fieldOptions.value.find((o) => o.value === value)?.label ?? value;
}

function addEnumValue(value: string) {
  if (!value) return;
  if (selectedEnumValues.value.includes(value)) return;
  props.predicate.enumValues = [...selectedEnumValues.value, value];
}

function removeEnumValue(value: string) {
  props.predicate.enumValues = selectedEnumValues.value.filter((v) => v !== value);
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

    <!-- Enum: op picker + chip-list of selected values + dropdown to add -->
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
        <span
          v-for="v in selectedEnumValues"
          :key="v"
          class="expr-pred-enum-chip"
        >
          {{ labelForEnumValue(v) }}
          <button
            type="button"
            class="expr-pred-enum-chip-remove"
            :title="t('workspace.templates.expression_builder.predicate.remove_value')"
            @click="removeEnumValue(v)"
          >×</button>
        </span>
        <SelectField
          v-if="addableEnumOptions.length > 0"
          :model-value="''"
          :options="addableEnumOptions"
          :placeholder="t('workspace.templates.expression_builder.predicate.add_value')"
          class="expr-pred-enum-add"
          @update:model-value="addEnumValue"
        />
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
        @change="setNumberValue(Number(($event.target as HTMLInputElement).value))"
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
        @change="setDateArg(Number(($event.target as HTMLInputElement).value))"
      />
    </template>
  </div>
</template>
