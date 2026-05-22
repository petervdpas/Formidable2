<script setup lang="ts">
import { ref } from "vue";

// FormSection - framed group of FormRows. When `collapsible` is
// set, the title becomes a clickable header that hides/shows the
// slot content. The grid is preserved (rows still use subgrid for
// label/control alignment); collapsing toggles a CSS class that
// the stylesheet uses to hide the row children.
const props = withDefaults(
  defineProps<{
    title?: string;
    /** Small muted hint rendered next to the title (always visible -
     *  particularly useful for collapsible sections so the user
     *  knows what's inside without expanding). */
    subtitle?: string;
    collapsible?: boolean;
    defaultCollapsed?: boolean;
  }>(),
  { collapsible: false, defaultCollapsed: false },
);

const collapsed = ref<boolean>(props.collapsible && props.defaultCollapsed);

function toggle() {
  if (!props.collapsible) return;
  collapsed.value = !collapsed.value;
}
</script>

<template>
  <section :class="['form-section', { 'form-section--collapsed': collapsible && collapsed }]">
    <h2
      v-if="title"
      :class="['form-section-title', { 'form-section-title--toggle': collapsible }]"
      :role="collapsible ? 'button' : undefined"
      :tabindex="collapsible ? 0 : undefined"
      :aria-expanded="collapsible ? !collapsed : undefined"
      @click="toggle"
      @keydown.enter.prevent="toggle"
      @keydown.space.prevent="toggle"
    >
      <span v-if="collapsible" class="form-section-chevron" aria-hidden="true">
        {{ collapsed ? '▶' : '▼' }}
      </span>
      <span>{{ title }}</span>
      <span v-if="subtitle" class="form-section-subtitle">{{ subtitle }}</span>
    </h2>
    <slot />
  </section>
</template>
