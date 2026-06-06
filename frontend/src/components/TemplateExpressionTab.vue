<script setup lang="ts">
/*
 * TemplateExpressionTab - the Sidebar Expression setup tab. The textarea is
 * read-only; the source is edited only through the visual builder (so the strict
 * round-trip parser stays happy). Owns the builder modal, the parse-check, and
 * the inline legacy-convert. Presentational: parent owns the draft; this emits
 * update:sidebarExpression.
 */
import { ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { TextareaField } from "./fields";
import ExpressionBuilderModal from "./ExpressionBuilderModal.vue";
import { Service as ExpressionSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/expression";
import type { FieldRef } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/expression/builder";
import type {
  Field,
  Facet,
  Formula,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { backendErrMessage } from "../utils/backendError";
import { useToast } from "../composables/useToast";

const props = defineProps<{
  sidebarExpression: string;
  fields: Field[];
  facets: Facet[];
  formulas: Formula[];
}>();
const emit = defineEmits<{ (e: "update:sidebarExpression", v: string): void }>();

const { t } = useI18n();
const toast = useToast();

const builderOpen = ref(false);
const parseable = ref(true);

function fieldRefs(): FieldRef[] {
  return (props.fields ?? [])
    .filter((f) => {
      if (!f.expression_item) return false;
      const tt = (f.type || "").toLowerCase();
      return tt !== "loopstart" && tt !== "loopstop" && tt !== "looper";
    })
    .map((f) => ({
      key: f.key,
      type: f.type || "",
      options: ((f.options ?? []) as any[]).map((o) => ({
        value: String(o?.value ?? ""),
        label: String(o?.label ?? o?.value ?? ""),
      })),
    }));
}

async function recheck() {
  const src = (props.sidebarExpression ?? "").trim();
  if (!src) {
    parseable.value = true;
    return;
  }
  try {
    await ExpressionSvc.BuilderParse(src, fieldRefs());
    parseable.value = true;
  } catch {
    parseable.value = false;
  }
}
watch(() => props.sidebarExpression, () => void recheck(), { immediate: true });

function applyBuilder(source: string) {
  builderOpen.value = false;
  emit("update:sidebarExpression", source);
}
function clearSource() {
  emit("update:sidebarExpression", "");
}
async function onConvert() {
  const src = (props.sidebarExpression ?? "").trim();
  if (!src) return;
  try {
    const migrated = await ExpressionSvc.BuilderConvert(src, fieldRefs());
    emit("update:sidebarExpression", migrated);
    toast.success("workspace.templates.expression_builder.convert_succeeded");
  } catch (err) {
    toast.error("workspace.templates.expression_builder.convert_failed");
    const detail = backendErrMessage(err);
    if (detail) toast.error(detail);
  }
}
</script>

<template>
  <div class="setup-tab-pane">
    <TextareaField
      :model-value="sidebarExpression"
      :rows="6"
      :readonly="true"
    />
    <div class="setup-tab-actions">
      <button class="tool-btn" type="button" @click="builderOpen = true">
        {{ t('workspace.templates.expression_builder.button') }}
      </button>
      <button
        v-if="!parseable"
        class="tool-btn primary"
        type="button"
        :title="t('workspace.templates.expression_builder.convert_title')"
        @click="onConvert"
      >
        {{ t('workspace.templates.expression_builder.convert') }}
      </button>
    </div>

    <ExpressionBuilderModal
      :open="builderOpen"
      :fields="fields"
      :facets="facets"
      :formulas="formulas"
      :initial="sidebarExpression"
      @close="builderOpen = false"
      @apply="applyBuilder"
      @clear="clearSource"
    />
  </div>
</template>
