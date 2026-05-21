<script setup lang="ts">
import { computed, watch } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import TreeView, { type TreeNode } from "../components/TreeView.vue";
import { useRestartGate } from "../composables/useRestartGate";
import { useConfig } from "../composables/useConfig";
import { useInformationSection } from "../composables/useInformationSection";
import { setTopbarMenu } from "../composables/useTopbarMenu";
import { useWorkspacePluginMenu } from "../composables/useWorkspacePluginMenu";
import {
  INFORMATION_CATEGORIES,
  type InformationCategory,
  findCategory,
  flattenLeaves,
} from "./information";

const { t } = useI18n();
const { bootConfig } = useRestartGate();
const { config } = useConfig();
const { active: activeId, setActive } = useInformationSection();

const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

// Recursively filter the (possibly nested) tree against the current
// config snapshot so dev/logging-only entries don't appear when
// disabled. Branches whose subtree becomes empty are dropped.
function filterTree(list: InformationCategory[]): InformationCategory[] {
  const out: InformationCategory[] = [];
  for (const c of list) {
    if (c.available && !c.available(config.value)) continue;
    if (c.children) {
      const filteredChildren = filterTree(c.children);
      if (filteredChildren.length === 0) continue;
      out.push({ ...c, children: filteredChildren });
    } else {
      out.push(c);
    }
  }
  return out;
}

const visibleTree = computed(() => filterTree(INFORMATION_CATEGORIES));
const visibleLeaves = computed(() => flattenLeaves(visibleTree.value));

// If the active entry becomes unavailable (user just turned the
// feature off while sitting on it), bounce to the first visible leaf.
watch(visibleLeaves, (leaves) => {
  if (!leaves.find((c) => c.id === activeId.value)) {
    setActive(leaves[0]?.id ?? "about");
  }
});

const activeCategory = computed(() => {
  const hit = findCategory(visibleTree.value, activeId.value);
  if (hit && hit.component) return hit;
  return visibleLeaves.value[0];
});

// Project the InformationCategory tree into TreeView's shape. Labels
// are translated here so the tree component stays presentation-only.
function toTreeNodes(list: InformationCategory[]): TreeNode[] {
  return list.map((c) => ({
    id: c.id,
    label: t(c.labelKey),
    children: c.children ? toTreeNodes(c.children) : undefined,
  }));
}

const treeItems = computed(() => toTreeNodes(visibleTree.value));

const { buildMenu: buildPluginsMenu } = useWorkspacePluginMenu("information");
setTopbarMenu(() => {
  const plugins = buildPluginsMenu();
  return plugins ? [plugins] : [];
});
</script>

<template>
  <Teleport defer to="#topbar-content">
    <span class="topbar-spacer"></span>
  </Teleport>

  <SplitPane :initial="sidebarWidth">
    <template #sidebar>
      <h2 class="sidebar-title">{{ t('workspace.information.sidebar_title') }}</h2>
      <TreeView
        :items="treeItems"
        :selected-id="activeId"
        @update:selected-id="(id) => setActive(id)"
      />
    </template>

    <template #main>
      <h1 class="workspace-heading">{{ t(activeCategory.labelKey) }}</h1>
      <component :is="activeCategory.component" v-bind="activeCategory.props || {}" />
    </template>
  </SplitPane>
</template>
