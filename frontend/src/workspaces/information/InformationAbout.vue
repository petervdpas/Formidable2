<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useI18n } from "vue-i18n";
import { Service as About, Info, Library } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/about";

const { t } = useI18n();
const info = ref<Info | null>(null);
// Canonical credits list. Go owns the order, the IDs, and the
// display names; i18n provides the per-locale description per ID.
// See about.Libraries in internal/modules/about/about.go.
const libraries = ref<Library[]>([]);

onMounted(async () => {
  info.value = await About.GetInfo();
  libraries.value = await About.GetLibraries();
});
</script>

<template>
  <div class="about-card" v-if="info">
    <div class="logo">
      <img src="/formidable.svg" alt="" />
    </div>
    <div class="identity">
      <div class="name">{{ info.name }}</div>
      <div class="tagline">{{ t('workspace.information.about.tagline') }}</div>
      <div class="version">{{ t('workspace.information.about.version', [info.version]) }}</div>
      <div class="author">{{ t('workspace.information.about.author', [info.author]) }}</div>
    </div>
  </div>

  <section class="about-section">
    <div class="text">
      <i18n-t keypath="workspace.information.about.elly_text" tag="span">
        <template #name>
          <strong>{{ t('workspace.information.about.elly_name') }}</strong>
        </template>
      </i18n-t>
    </div>
    <div class="quote">{{ t('workspace.information.about.elly_quote') }}</div>
  </section>

  <section class="about-section">
    <div class="text">
      <i18n-t keypath="workspace.information.about.aaron_text" tag="span">
        <template #name>
          <strong>{{ t('workspace.information.about.aaron_name') }}</strong>
        </template>
      </i18n-t>
    </div>
    <div class="quote">{{ t('workspace.information.about.aaron_quote') }}</div>
  </section>

  <section v-if="libraries.length" class="about-section about-thanks">
    <div class="text"><strong>{{ t('workspace.information.about.thanks.title') }}</strong></div>
    <div class="text">{{ t('workspace.information.about.thanks.intro') }}</div>
    <ul class="thanks-list">
      <li v-for="lib in libraries" :key="lib.id">
        <strong>{{ lib.name }}</strong>
        <span class="thanks-sep"> — </span>
        <span>{{ t(`workspace.information.about.thanks.lib.${lib.id}.desc`) }}</span>
      </li>
    </ul>
  </section>
</template>
