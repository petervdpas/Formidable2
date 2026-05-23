<script setup lang="ts">
import { useI18n } from "vue-i18n";
import Modal from "../Modal.vue";
import StatChart from "./StatChart.vue";
import type { ChartEnvelope } from "./types";

// Glance-and-close dialog that renders the chart(s) a plugin returned.
// One StatChart per envelope; each gets its own title heading when the
// plugin supplied one. Presentation only - all data arrived in `charts`.
const props = defineProps<{
  open: boolean;
  title?: string;
  charts: ChartEnvelope[];
}>();

const emit = defineEmits<{ close: [] }>();
const { t } = useI18n();
</script>

<template>
  <Modal
    :open="props.open"
    :title="props.title || t('workspace.stat.dialog.title')"
    width="600px"
    @close="emit('close')"
  >
    <div class="stat-dialog-body">
      <section
        v-for="(c, i) in props.charts"
        :key="i"
        class="stat-dialog-chart"
      >
        <h3 v-if="c.title" class="stat-dialog-chart-title">{{ c.title }}</h3>
        <StatChart :result="c.result" :type="c.type" />
      </section>
    </div>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('close')">
        {{ t('common.close') }}
      </button>
    </template>
  </Modal>
</template>
