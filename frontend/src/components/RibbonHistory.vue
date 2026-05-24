<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { Events } from "@wailsio/runtime";
import { Service as HistorySvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/history";
import { useActiveWorkspace } from "../composables/useActiveWorkspace";
import { useConfig } from "../composables/useConfig";
import { confirmLeave } from "../composables/useNavGuard";

const { t } = useI18n();
const { active } = useActiveWorkspace();
const { config } = useConfig();

// Back/forward only addresses storage entries (formidable://tpl:entry),
// and respects the user's master History toggle in Settings → History.
// Treat `enabled === undefined` as on so existing profiles without the
// field don't suddenly lose the chevrons.
const visible = computed(
  () => active.value === "storage" && config.value?.history?.enabled !== false,
);

const canBack = ref(false);
const canForward = ref(false);

async function refresh() {
  const s = await HistorySvc.State();
  canBack.value = !!s?.can_back;
  canForward.value = !!s?.can_forward;
}

async function back() {
  if (!canBack.value) return;
  if (!(await confirmLeave())) return; // guard before moving the history pointer
  await HistorySvc.Back();
}

async function forward() {
  if (!canForward.value) return;
  if (!(await confirmLeave())) return;
  await HistorySvc.Forward();
}

type StatePayload = { can_back?: boolean; can_forward?: boolean };

let unsub: (() => void) | null = null;
onMounted(() => {
  void refresh();
  unsub = Events.On("history:state", (ev: { data?: StatePayload } | StatePayload) => {
    const data = (ev as { data?: StatePayload })?.data ?? (ev as StatePayload);
    canBack.value = !!data?.can_back;
    canForward.value = !!data?.can_forward;
  });
});
onBeforeUnmount(() => {
  unsub?.();
  unsub = null;
});
</script>

<template>
  <div v-if="visible" class="ribbon-history">
    <button
      type="button"
      class="ribbon-history__btn"
      :disabled="!canBack"
      :aria-label="t('ribbon.history.back')"
      :title="t('ribbon.history.back')"
      @click="back"
    >&lsaquo;</button>
    <button
      type="button"
      class="ribbon-history__btn"
      :disabled="!canForward"
      :aria-label="t('ribbon.history.forward')"
      :title="t('ribbon.history.forward')"
      @click="forward"
    >&rsaquo;</button>
  </div>
</template>
