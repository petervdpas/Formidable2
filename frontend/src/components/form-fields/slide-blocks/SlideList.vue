<script setup lang="ts">
// Reveal "list" element: reuses the standard list field; the canvas shows the
// backend-rendered <ul>. SlideStyleControls gives it font/colour, and a List type
// dropdown numbers it. Numbered matches the prose renderer (decimal, then
// lower-alpha nested) via a block class, so slides and documents read the same.
import { computed, provide } from "vue";
import { useI18n } from "vue-i18n";
import FormFieldList from "../FormFieldList.vue";
import SlideRenderedPreview from "./SlideRenderedPreview.vue";
import SlideStyleControls from "./SlideStyleControls.vue";
import { syntheticField, type SlideBlock } from "../../../types/slide-blocks";
import { FORM_FIELD_OPS_KEY } from "../../../composables/formFieldOps";

const props = defineProps<{ block: SlideBlock; surface: "canvas" | "inspector"; html?: string }>();
const emit = defineEmits<{ (e: "patch", p: Partial<SlideBlock>): void }>();

const { t } = useI18n();

// A slide list is a block, not a saved record field, so the backend sort/dedup
// field-ops (keyed by a real field key) can't apply to it. Suppress them so those
// buttons hide instead of erroring on the block's synthetic id.
provide(FORM_FIELD_OPS_KEY, null);

const field = computed(() => syntheticField(props.block.id, "list"));

// Explicit key map (never interpolate the i18n key).
const LIST_TYPES = [
  { value: "bulleted", labelKey: "workspace.storage.slide.list_type.bulleted" },
  { value: "numbered", labelKey: "workspace.storage.slide.list_type.numbered" },
] as const;

const listType = computed(() => (props.block.ordered ? "numbered" : "bulleted"));
function setListType(v: string) {
  emit("patch", { ordered: v === "numbered" });
}
</script>

<template>
  <SlideRenderedPreview v-if="surface === 'canvas'" :block="block" :html="html" />
  <template v-else>
    <FormFieldList
      :field="field" :model-value="block.content"
      @update:model-value="emit('patch', { content: $event })"
    />
    <label class="slide-inspector-row">
      {{ t('workspace.storage.slide.list_type') }}
      <select :value="listType" @change="setListType(($event.target as HTMLSelectElement).value)">
        <option v-for="lt in LIST_TYPES" :key="lt.value" :value="lt.value">{{ t(lt.labelKey) }}</option>
      </select>
    </label>
    <SlideStyleControls :block="block" @patch="emit('patch', $event)" />
  </template>
</template>
