<script setup lang="ts">
/*
 * StatusGiGotQuick — footer indicator for the Gigot backend. Today
 * the Gigot module isn't built yet, so this is a minimal jump-button:
 * a cloud icon that routes to the Collaboration → current-service
 * page. When a future internal/modules/collaboration/gigot module
 * ships and a "gigot-sync" entry lands in COLLABORATION_SECTIONS,
 * swap the setSection target here and add poll-state to drive a
 * spinner — no other call site needs to change.
 */
import { useI18n } from "vue-i18n";
import { useActiveWorkspace } from "../composables/useActiveWorkspace";
import { useCollaborationSection } from "../composables/useCollaborationSection";

const { t } = useI18n();
const { setActive: setWorkspace } = useActiveWorkspace();
const { setActive: setSection } = useCollaborationSection();

function onClick() {
  setWorkspace("collaboration");
  // No "gigot-sync" section yet — land on the backend-agnostic
  // overview row. Update once the Gigot module brings its own.
  setSection("current-service");
}
</script>

<template>
  <button
    type="button"
    class="status-gigotquick"
    :title="t('statusbar.gigotquick.title')"
    :aria-label="t('statusbar.gigotquick.title')"
    @click="onClick"
  >
    <i class="fa-solid fa-cloud" aria-hidden="true"></i>
  </button>
</template>
