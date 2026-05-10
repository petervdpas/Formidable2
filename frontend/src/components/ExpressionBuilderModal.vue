<script setup lang="ts">
/*
 * ExpressionBuilderModal — visual builder for a template's
 * sidebar_expression. Skeleton: list of expression-flagged fields
 * on the left, per-field configure pane on the right. Configure
 * pieces (Display, State/rules, etc.) are added one slice at a
 * time so each step gets eyes on it before the next lands.
 *
 * The dialog is one-way: Apply overwrites the textarea. Round-trip
 * parsing of free-form expr-lang back into builder state is not
 * planned.
 */
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import Tabs, { type TabItem } from "./Tabs.vue";
import { SwitchField } from "./fields";
import type { Field } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  open: boolean;
  fields: Field[];
  initial?: string;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "apply", source: string): void;
}>();

const { t } = useI18n();

// Loop fences and the looper container don't carry per-row values,
// so they never become expression items even if mis-flagged.
const expressionFields = computed(() =>
  (props.fields ?? []).filter((f) => {
    if (!f.expression_item) return false;
    const t = (f.type || "").toLowerCase();
    return t !== "loopstart" && t !== "loopstop" && t !== "looper";
  }),
);

const selectedKey = ref<string>("");

const selectedField = computed<Field | null>(
  () => expressionFields.value.find((f) => f.key === selectedKey.value) ?? null,
);

// Per-field "Display" toggle. Lives in the configure-pane header
// (above the tabs) and gates the Display tab — a field doesn't enter
// the chip until the user opts it in. Defaults to off.
const canBeDisplayed = ref<Record<string, boolean>>({});

const displayAllowed = computed(() => {
  if (!selectedKey.value) return false;
  return !!canBeDisplayed.value[selectedKey.value];
});

// Per-field rules. One field can carry multiple rules; later
// slices fill in operator + value + styling. Today the row is just
// a placeholder so the add/remove plumbing has something to bind
// against.
type Rule = Record<string, never>;
const rulesByField = ref<Record<string, Rule[]>>({});

const currentRules = computed<Rule[]>(() => {
  if (!selectedKey.value) return [];
  return rulesByField.value[selectedKey.value] ?? [];
});

function addRule() {
  if (!selectedKey.value) return;
  const list = rulesByField.value[selectedKey.value] ?? [];
  rulesByField.value[selectedKey.value] = [...list, {}];
}

function removeRule(i: number) {
  if (!selectedKey.value) return;
  const list = rulesByField.value[selectedKey.value] ?? [];
  rulesByField.value[selectedKey.value] = [
    ...list.slice(0, i),
    ...list.slice(i + 1),
  ];
}

// Configure pane is split into horizontal tabs. State leads — it's
// where the rules tree lives — and Display is the styling-side
// second. Order matches the conceptual flow: decide what state the
// row is in, then decide how it looks.
const activeTab = ref<string>("state");

// Only field types that yield discrete or comparable values can
// drive rules. Anything else (text, lists, paths, guids…) gets the
// State tab disabled so authors aren't tempted to write a rule
// against a value that has no meaningful predicate.
const STATE_BEARING_TYPES = new Set([
  "boolean",
  "dropdown",
  "radio",
  "number",
  "range",
]);

const stateAvailable = computed(() => {
  const t = (selectedField.value?.type || "").toLowerCase();
  return STATE_BEARING_TYPES.has(t);
});

// Date routes to its own tab (helpers like isOverdue, normalizeDate,
// ageInDays). Transform handles value-level shaping for everything
// else — case, truncation, decimals, yes/no swap, option-label
// lookup, etc.
const isDate = computed(() => {
  const t = (selectedField.value?.type || "").toLowerCase();
  return t === "date";
});

const transformAvailable = computed(() => {
  if (!selectedField.value) return false;
  return !isDate.value;
});

const configTabs = computed<TabItem[]>(() => [
  {
    id: "state",
    label: t("workspace.templates.expression_builder.tab.state"),
    disabled: !stateAvailable.value,
  },
  {
    id: "date",
    label: t("workspace.templates.expression_builder.tab.date"),
    disabled: !isDate.value,
  },
  {
    id: "display",
    label: t("workspace.templates.expression_builder.tab.display"),
    disabled: !displayAllowed.value,
  },
  {
    id: "transform",
    label: t("workspace.templates.expression_builder.tab.transform"),
    disabled: !transformAvailable.value,
  },
]);

watch(
  () => props.open,
  (isOpen) => {
    if (!isOpen) return;
    // Reset per-field state so a previous open's choices don't bleed
    // into a fresh template selection.
    const flags: Record<string, boolean> = {};
    const rules: Record<string, Rule[]> = {};
    for (const f of expressionFields.value) {
      flags[f.key] = false;
      rules[f.key] = [];
    }
    canBeDisplayed.value = flags;
    rulesByField.value = rules;
    selectedKey.value = expressionFields.value[0]?.key ?? "";
    activeTab.value = defaultTabForField();
  },
  { immediate: true },
);

// Switching fields lands on the rule-producer tab the new field
// supports (State for state-bearing types, Date for date), otherwise
// Transform — never a disabled tab.
watch(selectedKey, () => {
  activeTab.value = defaultTabForField();
});

// Flipping the Display toggle off while the Display tab is active
// would leave it stuck on a disabled tab; bounce back to whichever
// rule-producer (or Transform) is enabled.
watch(displayAllowed, (allowed) => {
  if (!allowed && activeTab.value === "display") {
    activeTab.value = defaultTabForField();
  }
});

function defaultTabForField(): string {
  if (stateAvailable.value) return "state";
  if (isDate.value) return "date";
  if (transformAvailable.value) return "transform";
  return "display";
}

function pickField(key: string) {
  selectedKey.value = key;
}

// Apply is gated until there's something to compile. Skeleton has no
// compile yet, so it stays disabled — wires up next slice.
const canApply = computed(() => false);

function onApply() {
  // Placeholder; no source yet to emit.
  emit("apply", "");
}
</script>

<template>
  <Modal
    :open="open"
    :title="t('workspace.templates.expression_builder.title')"
    width="820px"
    @close="emit('close')"
  >
    <p v-if="initial && initial.trim()" class="muted small expr-builder-warn">
      {{ t('workspace.templates.expression_builder.replaces_source') }}
    </p>

    <p
      v-if="!expressionFields.length"
      class="muted small expr-builder-empty"
    >
      {{ t('workspace.templates.expression_builder.no_fields') }}
    </p>

    <div v-else class="expr-builder-grid">
      <fieldset class="expr-builder-fieldset expr-builder-list-fieldset">
        <legend>{{ t('workspace.templates.expression_builder.fields_block') }}</legend>
        <ul class="expr-builder-list">
          <li
            v-for="f in expressionFields"
            :key="f.key"
            class="expr-builder-list-row"
            :class="{ selected: selectedKey === f.key }"
            @click="pickField(f.key)"
          >
            <span class="expr-builder-list-text">
              <span class="expr-builder-list-label">{{ f.label || f.key }}</span>
              <span class="expr-builder-list-meta muted small">
                {{ f.key }} — {{ (f.type || '').toUpperCase() }}
              </span>
            </span>
          </li>
        </ul>
      </fieldset>

      <fieldset class="expr-builder-fieldset expr-builder-config-fieldset">
        <legend>
          <template v-if="selectedField">
            {{ t('workspace.templates.expression_builder.configure_block_for', { name: selectedField.label || selectedField.key }) }}
          </template>
          <template v-else>
            {{ t('workspace.templates.expression_builder.configure_block') }}
          </template>
        </legend>

        <p
          v-if="!selectedField"
          class="muted small expr-builder-config-empty"
        >
          {{ t('workspace.templates.expression_builder.configure_hint') }}
        </p>

        <template v-else>
          <div class="expr-builder-config-head">
            <span class="expr-builder-config-head-label">
              {{ t('workspace.templates.expression_builder.field.can_be_displayed') }}
            </span>
            <SwitchField
              v-if="selectedKey"
              v-model="canBeDisplayed[selectedKey]"
              :on-label="t('common.on')"
              :off-label="t('common.off')"
            />
          </div>

          <Tabs v-model="activeTab" :items="configTabs">
          <template #state>
            <div class="expr-builder-rules">
              <header class="expr-builder-rules-head">
                <span class="expr-builder-rules-title">
                  {{ t('workspace.templates.expression_builder.state.rules') }}
                </span>
                <button
                  class="tool-btn"
                  type="button"
                  @click="addRule"
                >
                  {{ t('workspace.templates.expression_builder.state.add_rule') }}
                </button>
              </header>

              <p
                v-if="currentRules.length === 0"
                class="muted small expr-builder-rules-empty"
              >
                {{ t('workspace.templates.expression_builder.state.rules_empty') }}
              </p>

              <ul v-else class="expr-builder-rule-list">
                <li
                  v-for="(_, i) in currentRules"
                  :key="i"
                  class="expr-builder-rule-row"
                >
                  <span class="expr-builder-rule-label">
                    {{ t('workspace.templates.expression_builder.state.rule_n', { n: i + 1 }) }}
                  </span>
                  <button
                    class="expr-builder-rule-remove"
                    type="button"
                    :title="t('workspace.templates.expression_builder.state.remove_rule')"
                    @click="removeRule(i)"
                  >×</button>
                </li>
              </ul>
            </div>
          </template>
        </Tabs>
        </template>
      </fieldset>
    </div>

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
