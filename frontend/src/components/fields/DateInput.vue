<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { VueDatePicker } from "@vuepic/vue-datepicker";
import { enUS, nl } from "date-fns/locale";
import type { Locale } from "date-fns";

// DateInput — shared building block: VueDatePicker normalised to
// emit/accept ISO `YYYY-MM-DD` strings via v-model. Used by
// FormFieldDate (top-level) and FormFieldTable (date columns).

const model = defineModel<string>({ default: "" });

defineProps<{
  readonly?: boolean;
  disabled?: boolean;
}>();

const { locale } = useI18n();

const LOCALES: Record<string, Locale> = {
  en: enUS,
  "en-US": enUS,
  nl: nl,
};
const dpLocale = computed<Locale>(() => LOCALES[locale.value] ?? enUS);

const isoRe = /^(\d{4})-(\d{2})-(\d{2})$/;

function toDate(s: string): Date | null {
  const m = isoRe.exec(s);
  if (!m) return null;
  const y = Number(m[1]);
  const mo = Number(m[2]);
  const d = Number(m[3]);
  if (!y || !mo || !d) return null;
  return new Date(y, mo - 1, d);
}

function toISO(d: Date | null | undefined): string {
  if (!d || !(d instanceof Date) || isNaN(d.getTime())) return "";
  const y = d.getFullYear();
  const mo = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${mo}-${day}`;
}

const date = computed<Date | null>({
  get: () => toDate(model.value ?? ""),
  set: (d) => { model.value = toISO(d); },
});
</script>

<template>
  <div class="date-field">
    <VueDatePicker
      v-model="date"
      :locale="dpLocale"
      :time-config="{ enableTimePicker: false }"
      :clearable="!readonly"
      :disabled="disabled || readonly"
      :auto-apply="true"
      format="yyyy-MM-dd"
      :teleport="true"
      week-start="1"
    />
  </div>
</template>
