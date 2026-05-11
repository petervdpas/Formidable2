<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { Events } from "@wailsio/runtime";
import { Service as HistorySvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/history";

const { t } = useI18n();

const canBack = ref(false);
const canForward = ref(false);

async function refresh() {
  const s = await HistorySvc.State();
  canBack.value = !!s?.can_back;
  canForward.value = !!s?.can_forward;
}

async function back() {
  if (!canBack.value) return;
  await HistorySvc.Back();
}

async function forward() {
  if (!canForward.value) return;
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
  <div class="ribbon-history">
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
