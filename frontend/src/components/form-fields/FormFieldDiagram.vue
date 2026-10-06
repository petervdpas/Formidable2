<script setup lang="ts">
/*
 * FormFieldDiagram: a virtual field that draws its bound table. The backend
 * owns binding, layout and SVG; this sends the live draft (so unsaved table
 * edits show) and displays the result. Click to open it zoomable.
 */
import { computed, inject, onBeforeUnmount, ref, watch, type Ref } from "vue";
import { useI18n } from "vue-i18n";
import ImageLightbox from "../ImageLightbox.vue";
import { Render } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/diagram/service";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { FORM_VALUES_KEY, type FormValuesContext } from "../../composables/formValues";
import { backendErrMessage } from "../../utils/backendError";

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const { t } = useI18n();

const templateFilename = inject<Ref<string>>("templateFilename", ref(""));
const formValues = inject<FormValuesContext | null>(FORM_VALUES_KEY, null);

const svg = ref("");
const error = ref("");
const lightboxOpen = ref(false);

const dataUrl = computed(() =>
  svg.value ? "data:image/svg+xml;charset=utf-8," + encodeURIComponent(svg.value) : "",
);

const valuesSignature = computed(() => JSON.stringify(formValues?.values.value ?? {}));

let timer: ReturnType<typeof setTimeout> | null = null;
let seq = 0;

async function draw() {
  if (!formValues || !templateFilename.value || !props.field.key) return;
  const mine = ++seq;
  try {
    const out = await Render(templateFilename.value, props.field.key, formValues.values.value);
    if (mine !== seq) return;
    svg.value = out;
    error.value = "";
  } catch (e) {
    if (mine !== seq) return;
    svg.value = "";
    error.value = backendErrMessage(e);
  }
}

watch(
  [valuesSignature, templateFilename, () => props.field],
  () => {
    if (timer) clearTimeout(timer);
    timer = setTimeout(draw, 250);
  },
  { immediate: true, deep: true },
);

onBeforeUnmount(() => {
  if (timer) clearTimeout(timer);
});
</script>

<template>
  <div class="diagram-field">
    <p v-if="!formValues" class="diagram-field-hint">{{ t("field.diagram.unavailable") }}</p>
    <p v-else-if="error" class="diagram-field-error">{{ error }}</p>
    <p v-else-if="!svg" class="diagram-field-hint">{{ t("field.diagram.empty") }}</p>
    <!-- Backend-built SVG: every label is XML-escaped and it carries no script or style. -->
    <div
      v-else
      class="diagram-field-canvas"
      :title="t('field.diagram.enlarge')"
      @click="lightboxOpen = true"
      v-html="svg"
    ></div>
    <ImageLightbox
      :open="lightboxOpen"
      :src="dataUrl"
      :alt="field.label || field.key"
      @close="lightboxOpen = false"
    />
  </div>
</template>
