<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import Tabs, { type TabItem } from "../../components/Tabs.vue";
import PDFCoversPanel from "./PDFCoversPanel.vue";
import PDFCoverImagesPanel from "./PDFCoverImagesPanel.vue";

const { t } = useI18n();

// Tab id persists at module scope so the user keeps their place when
// flipping between Information sections and returning here - matches
// usePDFCovers.ts's "preserve draft across navigation" stance.
const activeTab = ref<"covers" | "images">("covers");

const items = computed<TabItem[]>(() => [
  { id: "covers", label: t("pdf.covers.tab.covers") },
  { id: "images", label: t("pdf.covers.tab.images") },
]);
</script>

<template>
  <Tabs v-model="activeTab" :items="items">
    <template #covers>
      <PDFCoversPanel />
    </template>
    <template #images>
      <PDFCoverImagesPanel />
    </template>
  </Tabs>
</template>
