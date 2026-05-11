<script setup lang="ts">
import { computed } from "vue";
import Popup from "./Popup.vue";

export type SwatchOption = {
  /** Emitted value when this swatch is picked. */
  value: string;
  /** Tooltip + ARIA hint. Falls back to `value`. */
  label?: string;
  /** When the consumer colors via a class set (e.g. `flag-swatch-red`). */
  class?: string;
  /** When the consumer colors via an inline background (e.g. `#e84e4e`). */
  color?: string;
};

type Placement = "below" | "below-left" | "above" | "right" | "left";

const props = withDefaults(
  defineProps<{
    modelValue: string;
    options: SwatchOption[];
    /** Append an "× clear" cell that emits empty string. */
    clearable?: boolean;
    /** Tooltip on the × clear cell. */
    clearTitle?: string;
    placement?: Placement;
    /** Grid column count. */
    cols?: number;
    /** Cell side length (CSS value). */
    size?: string;
    /** Extra classes on the default trigger button (e.g. for sizing
     *  or context-specific borders). Ignored when #trigger is used. */
    triggerClass?: string;
    /** Tooltip on the default trigger. */
    triggerTitle?: string;
  }>(),
  {
    clearable: false,
    clearTitle: "Clear",
    placement: "right",
    cols: 4,
    size: "22px",
    triggerClass: "",
    triggerTitle: "",
  },
);

const emit = defineEmits<{
  (e: "update:modelValue", v: string): void;
}>();

const currentOption = computed(() =>
  props.options.find((o) => o.value === props.modelValue),
);

const gridStyle = computed(() => ({
  "--swatch-cols": String(props.cols),
  "--swatch-size": props.size,
}));

function pick(v: string, close: () => void) {
  emit("update:modelValue", v);
  close();
}
</script>

<template>
  <Popup :placement="placement">
    <template #trigger="slotProps">
      <slot
        name="trigger"
        :toggle="slotProps.toggle"
        :open="slotProps.open"
        :current="currentOption"
      >
        <button
          type="button"
          class="swatch-picker-trigger"
          :class="[triggerClass, currentOption?.class, { open: slotProps.open }]"
          :style="currentOption?.color ? { background: currentOption.color } : undefined"
          :title="triggerTitle || currentOption?.label || modelValue"
          @click="slotProps.toggle"
        ></button>
      </slot>
    </template>
    <template #default="{ close }">
      <div class="swatch-grid" :style="gridStyle" role="menu">
        <button
          v-for="opt in options"
          :key="opt.value"
          type="button"
          class="swatch-cell"
          :class="[opt.class, { active: opt.value === modelValue }]"
          :style="opt.color ? { background: opt.color } : undefined"
          :title="opt.label ?? opt.value"
          @click="pick(opt.value, close)"
        ></button>
        <button
          v-if="clearable"
          type="button"
          class="swatch-cell swatch-cell--clear"
          :title="clearTitle"
          @click="pick('', close)"
        >×</button>
      </div>
    </template>
  </Popup>
</template>
