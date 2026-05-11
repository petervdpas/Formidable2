<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { useConfig } from "../composables/useConfig";
import { useStatusBar } from "../composables/useStatusBar";

defineProps<{ profile?: string }>();

const { t } = useI18n();
const { config, profileFilename } = useConfig();
const {
  i18nKey: statusKey,
  i18nArgs: statusArgs,
  literal: statusLiteral,
  variant: statusVariant,
} = useStatusBar();

// Mirrors the fallback the Go side uses in ListAvailableProfiles:
// profile_name → author_name → filename → em-dash placeholder.
const activeProfileLabel = computed(() => {
  const name = config.value?.profile_name?.trim();
  if (name) return name;
  const author = config.value?.author_name?.trim();
  if (author) return author;
  if (profileFilename.value) return profileFilename.value;
  return t("footer.user_profile_unknown");
});

// Build a render-ready parts list from the i18n template. We let
// vue-i18n do the interpolation (so named/numbered/linked placeholders
// all behave correctly) but pass sentinel tokens as args; the regex
// then splits the result so we can wrap the real arg values in
// <strong> on the way out. Plain `t(key)` would erase `{0}` to empty
// before we ever saw it — that was the "Template saved successfully."
// bug.  is the SOH control char and won't collide with content.
type StatusPart = { text: string; arg: boolean };

const SENTINEL_RE = /(\d+)/g;

const statusParts = computed<StatusPart[]>(() => {
  if (!statusKey.value) return [];
  const args = statusArgs.value;
  const sentinels = args.map((_, i) => `${i}`);
  const interpolated = t(statusKey.value, sentinels as never);

  const parts: StatusPart[] = [];
  let last = 0;
  let m: RegExpExecArray | null;
  SENTINEL_RE.lastIndex = 0;
  while ((m = SENTINEL_RE.exec(interpolated)) !== null) {
    if (m.index > last) parts.push({ text: interpolated.slice(last, m.index), arg: false });
    const idx = parseInt(m[1], 10);
    const value = args[idx];
    parts.push({ text: value == null ? "" : String(value), arg: true });
    last = m.index + m[0].length;
  }
  if (last < interpolated.length) parts.push({ text: interpolated.slice(last), arg: false });
  return parts;
});
</script>

<template>
  <footer class="footer">
    <span class="status" :class="`status-${statusVariant}`">
      <template v-if="statusKey">
        <template v-for="(part, i) in statusParts" :key="i">
          <strong v-if="part.arg">{{ part.text }}</strong>
          <template v-else>{{ part.text }}</template>
        </template>
      </template>
      <template v-else-if="statusLiteral">{{ statusLiteral }}</template>
      <template v-else>{{ t("status.ready") }}</template>
    </span>
    <span class="footer-spacer" />
    <span class="profile muted">
      {{ t("footer.user_profile_label") }}
      {{ profile ?? activeProfileLabel }}
    </span>
  </footer>
</template>
