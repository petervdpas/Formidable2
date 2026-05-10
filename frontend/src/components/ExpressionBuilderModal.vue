<script setup lang="ts">
/*
 * ExpressionBuilderModal — visual builder for a template's
 * sidebar_expression. The data model lives backend-side in
 * `internal/modules/expression/builder` (Config / Rule / Predicate /
 * TextSource / Outcome) and reaches the frontend via Wails-generated
 * bindings — backend is the source of truth for the builder's
 * vocabulary so the dialog cannot drift from the engine.
 *
 * Currently in transition: the backend data model was reshaped from
 * per-field configs to a cross-field rule engine (rules AND
 * predicates over multiple fields, mapping to one styled chip).
 * The dialog UX is being rebuilt around that model in the next
 * slice. For now this file is a compile-clean stub that shows the
 * available expression_item fields and disables Apply.
 *
 * The dialog is one-way: Apply overwrites the textarea. Round-trip
 * parsing of free-form expr-lang back into builder state is not
 * planned.
 */
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
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
    const tt = (f.type || "").toLowerCase();
    return tt !== "loopstart" && tt !== "loopstop" && tt !== "looper";
  }),
);

const selectedKey = ref<string>("");

function pickField(key: string) {
  selectedKey.value = key;
}

// UX redesign in progress — Apply stays disabled until the new
// rule-list editor lands.
const canApply = computed(() => false);

function onApply() {
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
          {{ t('workspace.templates.expression_builder.configure_block') }}
        </legend>
        <p class="muted small expr-builder-config-empty">
          {{ t('workspace.templates.expression_builder.configure_hint') }}
        </p>
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
