<script setup lang="ts">
import MenuButton from "./MenuButton.vue";
import { useTopbarMenu } from "../composables/useTopbarMenu";

// The topbar has two halves:
//   • Left: a registry-driven menu list. Workspaces declare entries
//     via setTopbarMenu(); MenuButton dispatches each entry to either
//     a clickable leaf (no children) or a dropdown (children).
//   • Right: a Teleport target. Workspaces still own their action
//     markup directly so they can render arbitrary controls (badges,
//     toggles, etc.) without inflating the menu schema.
const { menus } = useTopbarMenu();
</script>

<template>
  <header class="topbar">
    <nav v-if="menus.length" class="topmenu" aria-label="Workspace menu">
      <MenuButton v-for="m in menus" :key="m.id" :entry="m" />
    </nav>
    <div id="topbar-content" class="topbar-content"></div>
  </header>
</template>
