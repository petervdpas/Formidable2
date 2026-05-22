<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Service as RenderSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/render";
import type { HelperDescriptor } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/render/models";

const { t } = useI18n();

const helpers = ref<HelperDescriptor[]>([]);
const loading = ref(true);
const activeCategory = ref<string>("");

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
// order so the tab strip matches the Go source.
const groups = computed(() => {
  const map = new Map<string, HelperDescriptor[]>();
  for (const h of helpers.value) {
    const key = String(h.category ?? "");
    const list = map.get(key) ?? [];
    list.push(h);
    map.set(key, list);
  }
  return Array.from(map.entries()).map(([category, items]) => ({ category, items }));
});

// Default-select the first category once helpers arrive. Persists the
// user's pick across re-renders within this session.
watch(groups, (g) => {
  if (!activeCategory.value && g.length > 0) {
    activeCategory.value = g[0].category;
  }
});

const activeItems = computed(() => {
  const g = groups.value.find((x) => x.category === activeCategory.value);
  return g?.items ?? [];
});

const CATEGORY_LABEL_KEYS: Record<string, string> = {
  api: "workspace.information.render_helpers.category.api",
  collection: "workspace.information.render_helpers.category.collection",
  comparison: "workspace.information.render_helpers.category.comparison",
  date: "workspace.information.render_helpers.category.date",
  field: "workspace.information.render_helpers.category.field",
  format: "workspace.information.render_helpers.category.format",
  image: "workspace.information.render_helpers.category.image",
  lookup: "workspace.information.render_helpers.category.lookup",
  loop: "workspace.information.render_helpers.category.loop",
  math: "workspace.information.render_helpers.category.math",
  meta: "workspace.information.render_helpers.category.meta",
  scratch: "workspace.information.render_helpers.category.scratch",
  stats: "workspace.information.render_helpers.category.stats",
  string: "workspace.information.render_helpers.category.string",
  tags: "workspace.information.render_helpers.category.tags",
};
function categoryLabel(category: string): string {
  const key = CATEGORY_LABEL_KEYS[category];
  if (!key) return category;
  const translated = t(key);
  return translated === key ? category : translated;
}
</script>

<template>
  <section class="render-helpers-page">
    <p class="muted small render-helpers-intro">{{ t('workspace.information.render_helpers.intro') }}</p>

    <p v-if="loading" class="muted small">{{ t('workspace.information.render_helpers.loading') }}</p>

    <div v-else class="tabs-container tabs-container--horizontal render-helpers-tabs">
      <nav class="tabs" role="tablist" aria-orientation="horizontal">
        <button
          v-for="g in groups"
          :key="g.category"
          type="button"
          role="tab"
          :class="['tab', { active: activeCategory === g.category }]"
          :aria-selected="activeCategory === g.category"
          @click="activeCategory = g.category"
        >
          {{ categoryLabel(g.category) }}
        </button>
      </nav>
      <section class="tab-pane" role="tabpanel">
        <ul class="render-helpers-list">
          <li v-for="h in activeItems" :key="h.name" class="render-helpers-card">
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
      </section>
    </div>
  </section>
</template>
