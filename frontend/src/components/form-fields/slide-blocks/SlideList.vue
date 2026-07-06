<script setup lang="ts">
// Reveal "list" element: reuses the standard list field; the canvas shows the
// backend-rendered <ul>/<ol>. SlideStyleControls gives it font/colour like the
// other text-bearing blocks.
import { computed, provide } from "vue";
import FormFieldList from "../FormFieldList.vue";
import SlideRenderedPreview from "./SlideRenderedPreview.vue";
import SlideStyleControls from "./SlideStyleControls.vue";
import { syntheticField, type SlideBlock } from "../../../types/slide-blocks";
import { FORM_FIELD_OPS_KEY } from "../../../composables/formFieldOps";

const props = defineProps<{ block: SlideBlock; surface: "canvas" | "inspector"; html?: string }>();
const emit = defineEmits<{ (e: "patch", p: Partial<SlideBlock>): void }>();

// A slide list is a block, not a saved record field, so the backend sort/dedup
// field-ops (keyed by a real field key) can't apply to it. Suppress them so those
// buttons hide instead of erroring on the block's synthetic id.
provide(FORM_FIELD_OPS_KEY, null);

const field = computed(() => syntheticField(props.block.id, "list"));
</script>

<template>
  <SlideRenderedPreview v-if="surface === 'canvas'" :block="block" :html="html" />
  <template v-else>
    <FormFieldList
      :field="field" :model-value="block.content"
      @update:model-value="emit('patch', { content: $event })"
    />
    <SlideStyleControls :block="block" @patch="emit('patch', $event)" />
  </template>
</template>
