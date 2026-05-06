<script setup lang="ts">
withDefaults(
  defineProps<{
    title?: string;
    handleLabel: string;
    offsetTop?: string;
  }>(),
  {
    offsetTop: "0px",
  },
);

const open = defineModel<boolean>("open", { default: false });

function toggle() {
  open.value = !open.value;
}

function close() {
  open.value = false;
}
</script>

<template>
  <div class="right-slideout">
    <button
      type="button"
      class="right-slideout-handle"
      :class="{ 'is-open': open }"
      :style="{ top: offsetTop }"
      :aria-expanded="open"
      :aria-label="handleLabel"
      @click="toggle"
    >
      <span class="right-slideout-handle-label">{{ handleLabel }}</span>
    </button>

    <Transition name="right-slideout">
      <aside
        v-if="open"
        class="right-slideout-panel"
        role="dialog"
        :aria-label="title"
      >
        <header class="right-slideout-header">
          <h3 class="right-slideout-title">{{ title }}</h3>
          <div class="right-slideout-header-actions">
            <slot name="header-actions" />
            <button
              type="button"
              class="right-slideout-close"
              aria-label="Close"
              @click="close"
            >×</button>
          </div>
        </header>
        <div class="right-slideout-body">
          <slot />
        </div>
      </aside>
    </Transition>
  </div>
</template>
