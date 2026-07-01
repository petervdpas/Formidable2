<script setup lang="ts">
// Deck picker for a multi-deck presentation template: choose which deck's
// slides the list shows (and reorders within). Mirrors StorageFacetFilter but
// over the template's slideset decks (backend-owned via FormSvc.Decks). "" =
// all decks (no per-deck scoping, reorder disabled).
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import Popup from "./Popup.vue";
import type { DeckOption } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/form";

const props = defineProps<{
  decks: DeckOption[];
  /** "" = all decks; otherwise the deck value to scope to. */
  modelValue: string;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: string): void }>();

const { t } = useI18n();

const active = computed(() => props.decks.find((d) => d.value === props.modelValue));
const triggerLabel = computed(() =>
  active.value ? active.value.label : t('workspace.storage.deck.all'),
);

function pick(value: string, close: () => void) {
  if (value !== props.modelValue) emit("update:modelValue", value);
  close();
}
</script>

<template>
  <Popup placement="below">
    <template #trigger="{ toggle, open }">
      <button
        type="button"
        class="facet-filter-trigger"
        :class="{ open, 'is-active': !!active }"
        @click="toggle"
      >
        <i class="fa-solid fa-layer-group" aria-hidden="true"></i>
        <span class="facet-filter-label">{{ triggerLabel }}</span>
      </button>
    </template>

    <template #default="{ close }">
      <div class="facet-picker-panel" role="menu">
        <button
          type="button"
          class="facet-picker-row"
          :class="{ active: !active }"
          role="menuitem"
          @click="pick('', close)"
        >
          <span class="facet-picker-label">{{ t('workspace.storage.deck.all') }}</span>
        </button>
        <button
          v-for="d in decks"
          :key="d.value"
          type="button"
          class="facet-picker-row"
          :class="{ active: d.value === modelValue }"
          role="menuitem"
          @click="pick(d.value, close)"
        >
          <span class="facet-picker-label">{{ d.label }}</span>
        </button>
      </div>
    </template>
  </Popup>
</template>
