<script setup lang="ts">
import { useI18n } from "vue-i18n";
import { SHORTCUT_CATALOG } from "../../utils/shortcutCatalog";
import { comboLabel, modifierLabel } from "../../utils/keyboardCombo";

const { t } = useI18n();
</script>

<template>
  <section class="shortcuts-page">
    <p class="muted small shortcuts-intro">
      {{ t('workspace.information.shortcuts.intro', [modifierLabel]) }}
    </p>

    <article v-for="group in SHORTCUT_CATALOG" :key="group.titleKey" class="shortcut-group">
      <h3 class="shortcut-group-title">{{ t(group.titleKey) }}</h3>
      <ul class="shortcut-list">
        <li v-for="item in group.items" :key="item.combo" class="shortcut-row">
          <kbd class="shortcut-combo">{{ comboLabel(item.combo) }}</kbd>
          <span class="shortcut-description">
            {{ t(item.descriptionKey) }}
            <span v-if="item.scopeKey" class="muted small shortcut-scope">
              - {{ t(item.scopeKey) }}
            </span>
          </span>
        </li>
      </ul>
    </article>
  </section>
</template>
