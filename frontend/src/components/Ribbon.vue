<script setup lang="ts">
import { onBeforeUnmount, onMounted } from "vue";
import { useI18n } from "vue-i18n";
import { Events } from "@wailsio/runtime";
import { WORKSPACES, type WorkspaceId } from "../workspaces";
import { useRibbonAvailability } from "../composables/useRibbonAvailability";
import { useToast } from "../composables/useToast";
import { Service as UpdateCheck } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/updatecheck";
import Icon from "./Icon.vue";
import RibbonHistory from "./RibbonHistory.vue";

defineProps<{ active: WorkspaceId }>();
const emit = defineEmits<{ (e: "select", id: WorkspaceId): void }>();

const { t } = useI18n();
const { isDisabled } = useRibbonAvailability();
const toast = useToast();

function onClick(id: WorkspaceId) {
  if (isDisabled(id)) return;
  emit("select", id);
}

// Startup release probe. Fires once, after the backend reveals the main
// window (main:shown) so the toast isn't painted behind the splash.
// CheckNow self-gates on the update_check config toggle and fails
// silently, so this is a no-op when the feature is off or offline.
let ran = false;
async function runStartupUpdateCheck() {
  if (ran) return;
  ran = true;
  try {
    const st = await UpdateCheck.CheckNow();
    if (!st.checked) return; // toggle off or probe failed: stay silent
    if (st.updateAvailable) {
      toast.warn(
        "workspace.information.about.update_available",
        ["v" + st.latest],
        { duration: 12000 },
      );
    } else {
      toast.success("workspace.information.about.up_to_date", undefined, {
        duration: 6000,
      });
    }
  } catch {
    // Silent by design: an update probe must never alarm the user.
  }
}

let unsubShown: (() => void) | null = null;
onMounted(() => {
  unsubShown = Events.On("main:shown", () => runStartupUpdateCheck());
});
onBeforeUnmount(() => {
  unsubShown?.();
  unsubShown = null;
});
</script>

<template>
  <nav class="ribbon" :aria-label="t('ribbon.settings')">
    <button
      v-for="w in WORKSPACES"
      :key="w.id"
      class="ribbon-item"
      :class="{ active: w.id === active, disabled: isDisabled(w.id) }"
      :title="t(w.labelKey)"
      :aria-label="t(w.labelKey)"
      :aria-current="w.id === active ? 'page' : undefined"
      :aria-disabled="isDisabled(w.id) ? 'true' : undefined"
      :disabled="isDisabled(w.id)"
      @click="onClick(w.id)"
    >
      <Icon :name="w.iconName" :size="36" />
    </button>
    <div class="ribbon-spacer" aria-hidden="true" />
    <RibbonHistory />
  </nav>
</template>
