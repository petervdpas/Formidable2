<script setup lang="ts">
/*
 * ReferenceCellPicker: a `reference` table cell, edited like the tags field
 * (type, Enter or comma adds a chip) but accepting only existing loop items.
 * The cell stores the codes as one "A, B" string, which renderers and the
 * diagram read. A stored code with no matching item stays visible, flagged,
 * until removed.
 */
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";

const props = defineProps<{
  modelValue: string;
  choices: string[];
  readonly?: boolean;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: string): void }>();

const { t } = useI18n();

const listId = "ref-" + Math.random().toString(36).slice(2, 10);

// Mirrors template.SplitReferenceCodes: split on , or ;, trim, dedupe case-insensitively.
const codes = computed<string[]>(() => {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const part of props.modelValue.split(/[,;]/)) {
    const c = part.trim();
    if (!c || seen.has(c.toLowerCase())) continue;
    seen.add(c.toLowerCase());
    out.push(c);
  }
  return out;
});

const byLower = computed(() => new Map(props.choices.map((c) => [c.toLowerCase(), c])));

const remaining = computed(() =>
  props.choices.filter((c) => !codes.value.some((x) => x.toLowerCase() === c.toLowerCase())),
);

const draft = ref("");
const invalid = ref(false);

function add() {
  const typed = draft.value.replace(/[,;]/g, "").trim();
  if (!typed) return;
  const match = byLower.value.get(typed.toLowerCase());
  if (!match) {
    invalid.value = true;
    return;
  }
  if (!codes.value.some((x) => x.toLowerCase() === match.toLowerCase())) {
    emit("update:modelValue", [...codes.value, match].join(", "));
  }
  draft.value = "";
  invalid.value = false;
}

function onInput() {
  invalid.value = false;
  if (/[,;]/.test(draft.value)) add();
}

function remove(i: number) {
  emit("update:modelValue", codes.value.filter((_, j) => j !== i).join(", "));
}
</script>

<template>
  <div class="tags-field reference-cell">
    <div v-if="codes.length" class="tag-row">
      <span
        v-for="(code, i) in codes"
        :key="code"
        :class="['tag-chip', { 'reference-chip--unknown': !byLower.has(code.toLowerCase()) }]"
        :title="byLower.has(code.toLowerCase()) ? undefined : t('workspace.storage.field.reference_unknown')"
      >
        <span>{{ code }}</span>
        <button
          v-if="!readonly"
          type="button"
          class="tag-remove"
          :aria-label="t('workspace.storage.field.reference_remove')"
          @click="remove(i)"
        >×</button>
      </span>
    </div>
    <template v-if="!readonly">
      <input
        v-if="choices.length"
        v-model="draft"
        :class="['field-input', 'tag-input', { invalid }]"
        :list="listId"
        :placeholder="t('workspace.storage.field.reference_add')"
        :title="invalid ? t('workspace.storage.field.reference_not_an_item') : undefined"
        @input="onInput"
        @keydown.enter.prevent="add"
      />
      <datalist :id="listId">
        <option v-for="c in remaining" :key="c" :value="c" />
      </datalist>
      <p v-if="!choices.length" class="reference-cell-hint">
        {{ t("workspace.storage.field.reference_no_items") }}
      </p>
    </template>
  </div>
</template>
