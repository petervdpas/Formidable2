<script setup lang="ts">
// Per-element typographic properties (size, colour, alignment, bold), stored in
// the block's style map and applied inline on the slide. Included only by the
// element types where text styling is meaningful (text/quote/math/code/table).
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import type { SlideBlock } from "../../../types/slide-blocks";

const props = withDefaults(
  defineProps<{ block: SlideBlock; align?: boolean; bold?: boolean }>(),
  { align: true, bold: true },
);
const emit = defineEmits<{ (e: "patch", p: Partial<SlideBlock>): void }>();

const { t } = useI18n();

function styleVal(prop: string): string {
  return props.block.style?.[prop] ?? "";
}
function setStyle(prop: string, val: string) {
  const s = { ...(props.block.style ?? {}) };
  if (val) s[prop] = val;
  else delete s[prop];
  emit("patch", { style: s });
}
const fontSize = computed(() => parseInt(styleVal("font-size"), 10) || 40);
const isBold = computed(() => styleVal("font-weight") === "bold");
</script>

<template>
  <div class="slide-style-grid">
    <label>{{ t('workspace.storage.slide.font_size') }}
      <input type="number" min="8" :value="fontSize" @input="setStyle('font-size', (($event.target as HTMLInputElement).value || '40') + 'px')" />
    </label>
    <div class="slide-style-color">
      <span>{{ t('workspace.storage.slide.color') }}</span>
      <input type="color" :value="styleVal('color') || '#000000'" @input="setStyle('color', ($event.target as HTMLInputElement).value)" />
      <button
        type="button" class="slide-style-clear" :class="{ active: !styleVal('color') }"
        :title="t('workspace.storage.slide.no_color')" @click="setStyle('color', '')"
      ><i class="fa-solid fa-ban" aria-hidden="true"></i></button>
    </div>
    <div class="slide-style-color">
      <span>{{ t('workspace.storage.slide.background') }}</span>
      <input type="color" :value="styleVal('background') || '#ffffff'" @input="setStyle('background', ($event.target as HTMLInputElement).value)" />
      <button
        type="button" class="slide-style-clear" :class="{ active: !styleVal('background') }"
        :title="t('workspace.storage.slide.no_color')" @click="setStyle('background', '')"
      ><i class="fa-solid fa-ban" aria-hidden="true"></i></button>
    </div>
    <div v-if="align || bold" class="slide-style-align">
      <span>{{ t('workspace.storage.slide.align') }}</span>
      <div class="slide-style-align-btns">
        <template v-if="align">
          <button type="button" :class="{ active: styleVal('text-align') === 'left' }" @click="setStyle('text-align', 'left')"><i class="fa-solid fa-align-left"></i></button>
          <button type="button" :class="{ active: styleVal('text-align') === 'center' }" @click="setStyle('text-align', 'center')"><i class="fa-solid fa-align-center"></i></button>
          <button type="button" :class="{ active: styleVal('text-align') === 'right' }" @click="setStyle('text-align', 'right')"><i class="fa-solid fa-align-right"></i></button>
        </template>
        <button v-if="bold" type="button" :class="{ active: isBold }" @click="setStyle('font-weight', isBold ? '' : 'bold')"><i class="fa-solid fa-bold"></i></button>
      </div>
    </div>
  </div>
</template>
