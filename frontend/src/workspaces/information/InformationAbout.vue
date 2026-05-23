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

const LIB_DESC_KEYS: Record<string, string> = {
  chroma: "workspace.information.about.thanks.lib.chroma.desc",
  codemirror: "workspace.information.about.thanks.lib.codemirror.desc",
  datepicker: "workspace.information.about.thanks.lib.datepicker.desc",
  easymde: "workspace.information.about.thanks.lib.easymde.desc",
  expr: "workspace.information.about.thanks.lib.expr.desc",
  fontawesome: "workspace.information.about.thanks.lib.fontawesome.desc",
  godog: "workspace.information.about.thanks.lib.godog.desc",
  gogit: "workspace.information.about.thanks.lib.gogit.desc",
  goldmark: "workspace.information.about.thanks.lib.goldmark.desc",
  gopherlua: "workspace.information.about.thanks.lib.gopherlua.desc",
  keyring: "workspace.information.about.thanks.lib.keyring.desc",
  pdfcpu: "workspace.information.about.thanks.lib.pdfcpu.desc",
  picoloom: "workspace.information.about.thanks.lib.picoloom.desc",
  raymond: "workspace.information.about.thanks.lib.raymond.desc",
  uuid: "workspace.information.about.thanks.lib.uuid.desc",
  vue: "workspace.information.about.thanks.lib.vue.desc",
  vuedraggable: "workspace.information.about.thanks.lib.vuedraggable.desc",
  vuei18n: "workspace.information.about.thanks.lib.vuei18n.desc",
  wails: "workspace.information.about.thanks.lib.wails.desc",
};
function libDesc(id: string): string {
  const key = LIB_DESC_KEYS[id];
  return key ? t(key) : "";
}

async function openWebsite() {
  await About.OpenWebsite();
}

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
      <a
        v-if="info.website"
        class="website"
        href="#"
        :title="t('workspace.information.about.website_open')"
        @click.prevent="openWebsite"
      >
        <i class="fa-solid fa-globe" aria-hidden="true"></i>
        <span>{{ info.website }}</span>
        <i class="fa-solid fa-arrow-up-right-from-square website-ext" aria-hidden="true"></i>
      </a>
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
        <span class="thanks-sep"> - </span>
        <span>{{ libDesc(lib.id) }}</span>
      </li>
    </ul>
  </section>
</template>
