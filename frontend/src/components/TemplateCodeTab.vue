<script setup lang="ts">
/*
 * TemplateCodeTab - the Template Code setup tab: the markdown/Handlebars editor
 * plus its validation status and the Generate button. Presentational: parent
 * owns the draft; this emits update:markdownTemplate, and a generate request the
 * parent opens the Generate dialog for.
 */
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import CodeEditor from "./CodeEditor.vue";
import { useTemplateValidation } from "../composables/useTemplateValidation";

const props = defineProps<{
  name: string;
  filename: string;
  markdownTemplate: string;
}>();
const emit = defineEmits<{
  (e: "update:markdownTemplate", v: string): void;
  (e: "generate"): void;
}>();

const { t } = useI18n();

const model = computed<string>({
  get: () => props.markdownTemplate ?? "",
  set: (v) => emit("update:markdownTemplate", v),
});

const {
  errorDiagnostic: templateErrorDiagnostic,
  warningDiagnostics: templateWarningDiagnostics,
  isOK: templateValidationOK,
} = useTemplateValidation(() => props.markdownTemplate ?? "");

const hasContent = computed(
  () => !!(props.markdownTemplate && props.markdownTemplate.trim()),
);
const editorTitle = computed(
  () => `${props.name || props.filename} • ${t('workspace.templates.setup.template_code')}`,
);
</script>

<template>
  <div class="setup-tab-pane">
    <CodeEditor
      v-model="model"
      lang="markdown"
      :height="180"
      :title="editorTitle"
    />
    <div v-if="hasContent" class="template-validate-status">
      <p
        v-if="templateErrorDiagnostic"
        class="template-validate-line error"
      >
        {{
          templateErrorDiagnostic.line
            ? t('workspace.templates.setup.validate.error_line', [String(templateErrorDiagnostic.line), templateErrorDiagnostic.message])
            : t('workspace.templates.setup.validate.error_no_line', [templateErrorDiagnostic.message])
        }}
      </p>
      <p
        v-for="(w, i) in templateWarningDiagnostics"
        :key="`warn-${i}-${w.helper}`"
        class="template-validate-line warning"
      >
        {{ w.message }}
      </p>
      <p
        v-if="!templateErrorDiagnostic && templateValidationOK"
        class="template-validate-line ok"
      >
        {{ t('workspace.templates.setup.validate.ok') }}
      </p>
    </div>
    <p class="muted small setup-tab-help">
      {{ t('workspace.templates.setup.template_code_help') }}
    </p>
    <div
      v-if="!hasContent"
      class="setup-tab-actions"
    >
      <button class="tool-btn" type="button" @click="emit('generate')">
        {{ t('workspace.templates.generate.button') }}
      </button>
    </div>
  </div>
</template>
