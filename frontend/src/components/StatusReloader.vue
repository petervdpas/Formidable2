<script setup lang="ts">
/*
 * StatusReloader — footer "↻" button that invokes the current
 * workspace's "Refresh" action. We don't reinvent the refresh path:
 * every workspace already registers a top-level menu group containing
 * a `{ id: "refresh", onClick: doRefresh }` entry, and we reach into
 * `useTopbarMenu` to find it. If the active workspace has no refresh
 * registered, the button greys out — better than a dead click.
 */
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { useTopbarMenu } from "../composables/useTopbarMenu";
import type { MenuAction } from "../types/menu";

const { t } = useI18n();
const { menus } = useTopbarMenu();

const refreshAction = computed<MenuAction | null>(() => {
  for (const entry of menus.value) {
    if (entry.type !== "group") continue;
    for (const item of entry.items) {
      if (item.type === "separator") continue;
      if (item.id === "refresh") return item;
    }
  }
  return null;
});

function reload() {
  const action = refreshAction.value;
  if (!action || action.disabled) return;
  void action.onClick();
}
</script>

<template>
  <button
    type="button"
    class="status-reloader"
    :disabled="!refreshAction || refreshAction.disabled"
    :title="t('statusbar.reload.title')"
    :aria-label="t('statusbar.reload.title')"
    @click="reload"
  >↻</button>
</template>
