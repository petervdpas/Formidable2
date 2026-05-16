<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { Service as RenderSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/render";
import type { HelperDescriptor } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/render/models";

const { t } = useI18n();

const helpers = ref<HelperDescriptor[]>([]);
const loading = ref(true);

onMounted(async () => {
  try {
    const list = await RenderSvc.ListHelpers();
    helpers.value = list ?? [];
  } finally {
    loading.value = false;
  }
});

// Group by category, preserving the catalog's source order (alphabetical
// inside each category) and listing categories in their first-seen
// order so the visual layout matches the Go source.
const grouped = computed(() => {
  const map = new Map<string, HelperDescriptor[]>();
  for (const h of helpers.value) {
    const key = String(h.category ?? "");
    const list = map.get(key) ?? [];
    list.push(h);
    map.set(key, list);
  }
  return Array.from(map.entries()).map(([category, items]) => ({ category, items }));
});

function categoryLabel(category: string): string {
  const key = `workspace.information.render_helpers.category.${category}`;
  const translated = t(key);
  return translated === key ? category : translated;
}
</script>

<template>
  <section class="render-helpers-page">
    <p class="muted small render-helpers-intro">{{ t('workspace.information.render_helpers.intro') }}</p>

    <p v-if="loading" class="muted small">{{ t('workspace.information.render_helpers.loading') }}</p>

    <article v-for="group in grouped" :key="group.category" class="render-helpers-group">
      <h3 class="render-helpers-group-title">{{ categoryLabel(group.category) }}</h3>
      <ul class="render-helpers-list">
        <li v-for="h in group.items" :key="h.name" class="render-helpers-card">
          <div class="render-helpers-name">
            <code>{{ h.name }}</code>
          </div>
          <p class="render-helpers-description">{{ h.description }}</p>
          <dl class="render-helpers-meta">
            <dt>{{ t('workspace.information.render_helpers.signature') }}</dt>
            <dd><code>{{ h.signature }}</code></dd>
            <dt>{{ t('workspace.information.render_helpers.example') }}</dt>
            <dd><code>{{ h.example }}</code></dd>
          </dl>
        </li>
      </ul>
    </article>
  </section>
</template>
