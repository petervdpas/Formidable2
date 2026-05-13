<script setup lang="ts">
import { computed } from "vue";
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

const next = computed<string>(() => {
  const list = availableLocales.value;
  if (list.length === 0) return current.value;
  const i = list.indexOf(current.value);
  return list[(i + 1) % list.length];
});

const label = computed(() => current.value.toUpperCase());

const tooltip = computed(() => {
  const cur = t("statusbar.language.current", [endonym(current.value)]);
  if (next.value === current.value) return cur;
  return `${cur} — ${t("statusbar.language.switch_to", [endonym(next.value)])}`;
});

function cycle() {
  if (next.value === current.value) return;
  void update({ language: next.value });
}
</script>

<template>
  <button
    type="button"
    class="status-language-quick"
    :disabled="availableLocales.length < 2"
    :title="tooltip"
    :aria-label="tooltip"
    @click="cycle"
  >{{ label }}</button>
</template>
