<script setup lang="ts">
/*
 * Tabs — reusable tab strip + active pane. Supports horizontal
 * (default) or vertical orientation. Pairs with `.tabs` / `.tab` /
 * `.tab-pane` (+ `.tabs-container--vertical` modifier) from
 * styles/tabs.css.
 *
 * Usage:
 *   <Tabs v-model="active" :items="[
 *     { id: 'a', label: 'A' },
 *     { id: 'b', label: 'B' },
 *   ]" orientation="horizontal">
 *     <template #a>…content A…</template>
 *     <template #b>…content B…</template>
 *   </Tabs>
 *
 * One slot per tab id; only the active tab's slot renders. Tabs
 * with `disabled: true` render but can't be activated.
 */

export type TabItem = {
  id: string;
  label: string;
  disabled?: boolean;
};

export type TabOrientation = "horizontal" | "vertical";

const props = withDefaults(
  defineProps<{
    modelValue: string;
    items: TabItem[];
    orientation?: TabOrientation;
  }>(),
  { orientation: "horizontal" },
);

const emit = defineEmits<{
  (e: "update:modelValue", id: string): void;
}>();

function pick(item: TabItem) {
  if (item.disabled) return;
  emit("update:modelValue", item.id);
}
</script>

<template>
  <div
    :class="[
      'tabs-container',
      props.orientation === 'vertical'
        ? 'tabs-container--vertical'
        : 'tabs-container--horizontal',
    ]"
  >
    <nav
      class="tabs"
      role="tablist"
      :aria-orientation="props.orientation"
    >
      <button
        v-for="item in items"
        :key="item.id"
        type="button"
        role="tab"
        :class="['tab', { active: modelValue === item.id }]"
        :aria-selected="modelValue === item.id"
        :disabled="item.disabled"
        @click="pick(item)"
      >
        {{ item.label }}
      </button>
    </nav>
    <section
      v-if="$slots[modelValue]"
      class="tab-pane"
      role="tabpanel"
    >
      <slot :name="modelValue" />
    </section>
  </div>
</template>
