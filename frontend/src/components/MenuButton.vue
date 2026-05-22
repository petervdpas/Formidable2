<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import DropdownMenu from "./DropdownMenu.vue";
import MenuItem from "./MenuItem.vue";
import MenuSeparator from "./MenuSeparator.vue";
import type { MenuEntry, MenuAction } from "../types/menu";

const props = defineProps<{ entry: MenuEntry }>();

const { t } = useI18n();

const isGroup = computed(() => props.entry.type === "group");

// A group is disabled when every actionable child is disabled -
// opening it would just show an empty list of greyed-out items, so we
// save the click. Separators don't count toward "actionable." Groups
// can opt out via `alwaysEnabled: true` if discoverability matters
// more than the saved click.
const groupDisabled = computed(() => {
  if (props.entry.type !== "group") return false;
  if (props.entry.alwaysEnabled) return false;
  const actions = props.entry.items.filter(
    (i): i is MenuAction => i.type !== "separator",
  );
  if (actions.length === 0) return true;
  return actions.every((a) => a.disabled === true);
});
</script>

<template>
  <DropdownMenu
    v-if="isGroup && entry.type === 'group'"
    :label="t(entry.labelKey)"
    :disabled="groupDisabled"
  >
    <template v-for="(it, i) in entry.items" :key="('id' in it && it.id) || i">
      <MenuSeparator v-if="it.type === 'separator'" />
      <MenuItem
        v-else
        :label="it.label ?? t(it.labelKey)"
        :hint="it.hintKey ? t(it.hintKey) : undefined"
        :combo="it.combo"
        :disabled="it.disabled"
        @click="it.onClick"
      />
    </template>
  </DropdownMenu>

  <button
    v-else-if="entry.type !== 'group'"
    type="button"
    class="topmenu-item"
    :disabled="entry.disabled"
    @click="entry.onClick"
  >
    {{ entry.label ?? t(entry.labelKey) }}
  </button>
</template>
