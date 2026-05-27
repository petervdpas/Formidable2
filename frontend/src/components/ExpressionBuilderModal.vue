<script setup lang="ts">
/*
 * ExpressionBuilderModal - visual builder for a template's
 * sidebar_expression. The data model lives backend-side in
 * `internal/modules/expression/builder` (Config / Rule / Predicate /
 * TextSource / Outcome) and reaches the frontend via Wails-generated
 * bindings - backend is the source of truth for the builder's
 * vocabulary so the dialog cannot drift from the engine.
 *
 * Layout: rule list on the left (with a Default pseudo-row at the
 * bottom), rule editor on the right (predicates + outcome). The
 * dialog is one-way - Apply overwrites the textarea; round-trip
 * parsing of free-form expr-lang is not planned.
 */
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import ScrollList from "./ScrollList.vue";
import PredicateRow from "./expressionBuilder/PredicateRow.vue";
import OutcomeEditor from "./expressionBuilder/OutcomeEditor.vue";
import type { Field } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { Service as ExpressionSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/expression";
import {
  Config,
  Outcome,
  type DateOpDescriptor,
  type FieldOption,
  type FieldRef,
  type Operator,
  type Rule,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/expression/builder";
import { backendErrMessage } from "../utils/backendError";

const props = defineProps<{
  open: boolean;
  fields: Field[];
  initial?: string;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "apply", source: string): void;
  /** Parent should clear the textarea without closing the dialog -
   * fired when an existing source can't be parsed back into a Config. */
  (e: "clear"): void;
}>();

const { t } = useI18n();

// ── Field-side derivations ──────────────────────────────────────

// Loop fences and the looper container don't carry per-row values,
// so they never become expression items even if mis-flagged.
const expressionFields = computed(() =>
  (props.fields ?? []).filter((f) => {
    if (!f.expression_item) return false;
    const tt = (f.type || "").toLowerCase();
    return tt !== "loopstart" && tt !== "loopstop" && tt !== "looper";
  }),
);

// Backend tells us which field types accept predicates; we filter
// against that list rather than hardcoding to keep parity with the
// engine's vocabulary.
const predicateableFields = ref<Field[]>([]);

async function refreshPredicateableFields() {
  const out: Field[] = [];
  for (const f of expressionFields.value) {
    const kind = await ExpressionSvc.BuilderKindForFieldType(f.type || "");
    if (kind) out.push(f);
  }
  predicateableFields.value = out;
}

const enumFields = computed(() =>
  expressionFields.value.filter((f) => {
    const tt = (f.type || "").toLowerCase();
    return tt === "dropdown" || tt === "radio";
  }),
);

function fieldByKey(key: string): Field | null {
  return expressionFields.value.find((f) => f.key === key) ?? null;
}

function fieldOptionsFor(key: string): FieldOption[] {
  const raw = (fieldByKey(key)?.options ?? []) as any[];
  return raw.map((o) => ({
    value: String(o?.value ?? ""),
    label: String(o?.label ?? o?.value ?? ""),
  }));
}

// ── Config state ────────────────────────────────────────────────

const config = ref<Config>(new Config({ rules: [], default: new Outcome() }));

// Session-scoped rule ids reset on modal open. Backend returns rules
// with empty IDs; the dialog assigns them so they remain stable
// across the editing session even though Apply is one-way.
let _ruleSeq = 0;
const newRuleId = () => `r${++_ruleSeq}`;

const selectedRuleId = ref<string>(""); // "" | "default" | <rule id>

const rules = computed<Rule[]>(() => config.value.rules ?? []);

const selectedRule = computed<Rule | null>(() => {
  const id = selectedRuleId.value;
  if (!id || id === "default") return null;
  return rules.value.find((r) => r.id === id) ?? null;
});

const editingDefault = computed(() => selectedRuleId.value === "default");

const editingOutcome = computed<Outcome | null>(() => {
  if (editingDefault.value) return config.value.default;
  return selectedRule.value?.outcome ?? null;
});

// ── Operator metadata (cached per session) ──────────────────────

const enumOps = ref<Operator[]>([]);
const numberOps = ref<Operator[]>([]);
const dateOps = ref<DateOpDescriptor[]>([]);

async function loadMetadata() {
  const [eOps, nOps, dOps] = await Promise.all([
    ExpressionSvc.BuilderOperatorsForKind("enum"),
    ExpressionSvc.BuilderOperatorsForKind("number"),
    ExpressionSvc.BuilderDateOps(),
  ]);
  enumOps.value = eOps;
  numberOps.value = nOps;
  dateOps.value = dOps;
}

// ── Open watcher: reset state, then preload from `initial` ─────

const parseError = ref<string>("");

watch(
  () => props.open,
  async (isOpen) => {
    if (!isOpen) return;
    _ruleSeq = 0;
    config.value = new Config({ rules: [], default: new Outcome() });
    selectedRuleId.value = "";
    applyError.value = "";
    parseError.value = "";
    addPredicateField.value = "";
    await Promise.all([loadMetadata(), refreshPredicateableFields()]);

    // Preload the existing sidebar_expression so the dialog opens on
    // the user's last-saved state. Strict parser - only the AST shape
    // Compile emits round-trips. On any failure we keep the dialog
    // empty, surface a warning, and ask the parent to wipe the
    // textarea so the unparseable source doesn't silently survive.
    const existing = (props.initial ?? "").trim();
    if (!existing) return;
    const fieldRefs: FieldRef[] = expressionFields.value.map((f) => ({
      key: f.key,
      type: f.type || "",
      options: fieldOptionsFor(f.key),
    }));
    try {
      const parsed = await ExpressionSvc.BuilderParse(existing, fieldRefs);
      config.value = parsed;
      // Walk parsed rules to advance the local id counter so newly-
      // added rules don't collide with parsed ones.
      const rs = parsed.rules ?? [];
      _ruleSeq = rs.length;
      // Land on the first rule (or default if there are none) so the
      // editor isn't staring at an empty placeholder.
      selectedRuleId.value = rs[0]?.id ?? "default";
    } catch (err) {
      parseError.value = backendErrMessage(err);
      emit("clear");
    }
  },
  { immediate: true },
);

// ── Rule operations ─────────────────────────────────────────────

async function addRule() {
  const r = await ExpressionSvc.BuilderDefaultRule();
  r.id = newRuleId();
  config.value.rules = [...(config.value.rules ?? []), r];
  selectedRuleId.value = r.id;
}

function removeRule(id: string) {
  config.value.rules = (config.value.rules ?? []).filter((r) => r.id !== id);
  if (selectedRuleId.value === id) selectedRuleId.value = "";
}

function selectRule(id: string) {
  selectedRuleId.value = id;
}

function selectDefault() {
  selectedRuleId.value = "default";
}

function ruleIndex(id: string): number {
  return rules.value.findIndex((r) => r.id === id);
}

function ruleSummary(r: Rule): string {
  const n = (r.predicates ?? []).length;
  if (n === 0) {
    return t("workspace.templates.expression_builder.rule_summary.always");
  }
  return t("workspace.templates.expression_builder.rule_summary.predicates", { n });
}

// ── Predicate operations (within selected rule) ─────────────────

const addPredicateField = ref<string>("");

async function onAddPredicateChange() {
  const key = addPredicateField.value;
  if (!key || !selectedRule.value) {
    addPredicateField.value = "";
    return;
  }
  const f = fieldByKey(key);
  if (!f) {
    addPredicateField.value = "";
    return;
  }
  try {
    const p = await ExpressionSvc.BuilderDefaultPredicate(f.type || "", f.key);
    selectedRule.value.predicates = [...(selectedRule.value.predicates ?? []), p];
  } catch (err) {
    applyError.value = backendErrMessage(err);
  } finally {
    addPredicateField.value = "";
  }
}

function removePredicate(i: number) {
  if (!selectedRule.value) return;
  const preds = selectedRule.value.predicates ?? [];
  selectedRule.value.predicates = [...preds.slice(0, i), ...preds.slice(i + 1)];
}

// ── Apply ───────────────────────────────────────────────────────

const applyError = ref<string>("");

async function onApply() {
  try {
    const fieldRefs: FieldRef[] = expressionFields.value.map((f) => ({
      key: f.key,
      type: f.type || "",
      options: fieldOptionsFor(f.key),
    }));
    const src = await ExpressionSvc.BuilderCompile(config.value, fieldRefs);
    applyError.value = "";
    emit("apply", src);
  } catch (err) {
    applyError.value = backendErrMessage(err);
  }
}

const canApply = computed(() => {
  // Allow apply even with an empty config - emits "" so the user can
  // clear an existing sidebar_expression. Backend rejects malformed
  // configs (kind mismatches, missing values) and we surface the
  // error inline rather than guess about completeness here.
  return true;
});
</script>

<template>
  <Modal
    :open="open"
    :title="t('workspace.templates.expression_builder.title')"
    width="900px"
    @close="emit('close')"
  >
    <p v-if="parseError" class="expr-builder-error small">
      {{ t('workspace.templates.expression_builder.parse_failed') }}
    </p>
    <p
      v-else-if="initial && initial.trim()"
      class="muted small expr-builder-warn"
    >
      {{ t('workspace.templates.expression_builder.replaces_source') }}
    </p>

    <p
      v-if="!expressionFields.length"
      class="muted small expr-builder-empty"
    >
      {{ t('workspace.templates.expression_builder.no_fields') }}
    </p>

    <div v-else class="expr-builder-grid">
      <!-- LEFT: rule list with Default pseudo-row -->
      <fieldset class="expr-builder-fieldset expr-builder-list-fieldset">
        <legend>{{ t('workspace.templates.expression_builder.rules.block') }}</legend>
        <button class="tool-btn expr-builder-add-rule" type="button" @click="addRule">
          {{ t('workspace.templates.expression_builder.rules.add') }}
        </button>
        <p
          v-if="!rules.length"
          class="muted small expr-builder-rules-empty"
        >
          {{ t('workspace.templates.expression_builder.rules.empty') }}
        </p>
        <ScrollList v-else max-height="36vh">
          <ul class="expr-builder-list">
            <li
              v-for="r in rules"
              :key="r.id"
              class="expr-builder-list-row"
              :class="{ selected: selectedRuleId === r.id }"
              @click="selectRule(r.id)"
            >
              <span class="expr-builder-list-text">
                <span class="expr-builder-list-label">
                  {{ t('workspace.templates.expression_builder.rule_n', { n: ruleIndex(r.id) + 1 }) }}
                </span>
                <span class="expr-builder-list-meta muted small">
                  {{ ruleSummary(r) }}
                </span>
              </span>
              <button
                class="expr-builder-rule-remove"
                type="button"
                :title="t('workspace.templates.expression_builder.rules.remove')"
                @click.stop="removeRule(r.id)"
              >×</button>
            </li>
          </ul>
        </ScrollList>
        <div
          class="expr-builder-default-row"
          :class="{ selected: editingDefault }"
          @click="selectDefault"
        >
          {{ t('workspace.templates.expression_builder.default_row') }}
        </div>
      </fieldset>

      <!-- RIGHT: rule editor -->
      <fieldset class="expr-builder-fieldset expr-builder-config-fieldset">
        <legend>
          <template v-if="selectedRule">
            {{ t('workspace.templates.expression_builder.rule_n', { n: ruleIndex(selectedRule.id) + 1 }) }}
          </template>
          <template v-else-if="editingDefault">
            {{ t('workspace.templates.expression_builder.default_row') }}
          </template>
          <template v-else>-</template>
        </legend>

        <p
          v-if="!selectedRule && !editingDefault"
          class="muted small expr-builder-config-empty"
        >
          {{ t('workspace.templates.expression_builder.no_rule_selected') }}
        </p>

        <template v-else>
          <!-- PREDICATES (only for actual rules - default has no predicates) -->
          <section v-if="selectedRule" class="expr-builder-predicates">
            <h4 class="expr-builder-section-title">
              {{ t('workspace.templates.expression_builder.predicates_block') }}
            </h4>
            <ScrollList
              v-if="(selectedRule.predicates ?? []).length"
              max-height="18vh"
            >
              <ul class="expr-builder-pred-list">
                <li
                  v-for="(p, i) in selectedRule.predicates"
                  :key="i"
                  class="expr-builder-pred-item"
                >
                  <PredicateRow
                    :predicate="p"
                    :field="fieldByKey(p.fieldKey)"
                    :enum-ops="enumOps"
                    :number-ops="numberOps"
                    :date-ops="dateOps"
                  />
                  <button
                    class="expr-builder-rule-remove"
                    type="button"
                    :title="t('workspace.templates.expression_builder.predicate.remove')"
                    @click="removePredicate(i)"
                  >×</button>
                </li>
              </ul>
            </ScrollList>
            <p v-else class="muted small">
              {{ t('workspace.templates.expression_builder.predicate.no_predicates') }}
            </p>
            <select
              class="expr-builder-add-predicate"
              v-model="addPredicateField"
              @change="onAddPredicateChange"
            >
              <option value="">
                {{ t('workspace.templates.expression_builder.predicate.add_for') }}
              </option>
              <option
                v-for="f in predicateableFields"
                :key="f.key"
                :value="f.key"
              >
                {{ f.label || f.key }}
              </option>
            </select>
          </section>

          <!-- OUTCOME -->
          <section v-if="editingOutcome" class="expr-builder-outcome">
            <h4 class="expr-builder-section-title">
              {{ t('workspace.templates.expression_builder.outcome.block') }}
            </h4>
            <OutcomeEditor
              :outcome="editingOutcome"
              :expression-fields="expressionFields"
              :enum-fields="enumFields"
            />
          </section>
        </template>
      </fieldset>
    </div>

    <p v-if="applyError" class="expr-builder-error small">{{ applyError }}</p>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('close')">
        {{ t('common.cancel') }}
      </button>
      <button
        class="tool-btn primary"
        type="button"
        :disabled="!canApply"
        @click="onApply"
      >
        {{ t('workspace.templates.expression_builder.apply') }}
      </button>
    </template>
  </Modal>
</template>
