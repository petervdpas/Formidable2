<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useConfig } from "../composables/useConfig";
import { useI18nLoader } from "../composables/useI18nLoader";

const { t } = useI18n();
const { config, update } = useConfig();
const { availableLocales } = useI18nLoader();

const ENDONYMS: Record<string, string> = {
  en: "English",
  nl: "Nederlands",
};

function endonym(loc: string): string {
  return ENDONYMS[loc] ?? loc.toUpperCase();
}

const current = computed<string>(() => {
  const lang = config.value?.language;
  if (lang && availableLocales.value.includes(lang)) return lang;
  return availableLocales.value[0] ?? "en";
});

const label = computed(() => current.value.toUpperCase());
const tooltip = computed(() => t("statusbar.language.current", [endonym(current.value)]));

const open = ref(false);
const triggerRef = ref<HTMLButtonElement | null>(null);
const popoverRef = ref<HTMLDivElement | null>(null);

function toggle() {
  open.value = !open.value;
}

function close() {
  open.value = false;
}

function pick(loc: string) {
  close();
  if (loc === current.value) return;
  void update({ language: loc });
}

function onDocClick(e: MouseEvent) {
  if (!open.value) return;
  const target = e.target as Node | null;
  if (!target) return;
  if (popoverRef.value?.contains(target)) return;
  if (triggerRef.value?.contains(target)) return;
  close();
}
function onKeyDown(e: KeyboardEvent) {
  if (open.value && e.key === "Escape") {
    e.stopPropagation();
    close();
    triggerRef.value?.focus();
  }
}

document.addEventListener("mousedown", onDocClick, true);
document.addEventListener("keydown", onKeyDown);
onBeforeUnmount(() => {
  document.removeEventListener("mousedown", onDocClick, true);
  document.removeEventListener("keydown", onKeyDown);
});
</script>

<template>
  <div class="status-language">
    <button
      ref="triggerRef"
      type="button"
      class="status-language-quick"
      :class="{ 'is-open': open }"
      :disabled="availableLocales.length < 2"
      :title="tooltip"
      :aria-label="tooltip"
      :aria-haspopup="true"
      :aria-expanded="open"
      @click="toggle"
    >{{ label }}</button>

    <div
      v-if="open"
      ref="popoverRef"
      class="status-language-popover"
      role="menu"
      :aria-label="t('statusbar.language.menu')"
    >
      <button
        v-for="loc in availableLocales"
        :key="loc"
        type="button"
        role="menuitemradio"
        :aria-checked="loc === current"
        class="status-language-item"
        :class="{ active: loc === current }"
        @click="pick(loc)"
      >
        <span class="status-language-item-code">{{ loc.toUpperCase() }}</span>
        <span class="status-language-item-name">{{ endonym(loc) }}</span>
      </button>
    </div>
  </div>
</template>
