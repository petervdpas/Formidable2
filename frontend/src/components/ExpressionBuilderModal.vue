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
  "date",
]);

const stateAvailable = computed(() => {
  const t = (selectedField.value?.type || "").toLowerCase();
  return STATE_BEARING_TYPES.has(t);
});

const configTabs = computed<TabItem[]>(() => [
  {
    id: "state",
    label: t("workspace.templates.expression_builder.tab.state"),
    disabled: !stateAvailable.value,
  },
  {
    id: "display",
    label: t("workspace.templates.expression_builder.tab.display"),
  },
]);

watch(
  () => props.open,
  (isOpen) => {
    if (!isOpen) return;
    selectedKey.value = expressionFields.value[0]?.key ?? "";
    activeTab.value = stateAvailable.value ? "state" : "display";
  },
  { immediate: true },
);

// Switching fields lands on State when the new field supports it,
// otherwise drops to Display so the user never stares at a disabled
// active tab.
watch(selectedKey, () => {
  activeTab.value = stateAvailable.value ? "state" : "display";
});

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

        <Tabs v-else v-model="activeTab" :items="configTabs">
          <!-- Tab content lands here, one slice at a time. -->
        </Tabs>
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
