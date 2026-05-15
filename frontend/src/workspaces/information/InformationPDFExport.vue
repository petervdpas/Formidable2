<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import { usePDFActivation } from "../../composables/usePDFActivation";
import { useToast } from "../../composables/useToast";
import Modal from "../../components/Modal.vue";
import { Service as DialogSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/dialog";
import type { ChromeCandidate } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/pdf/models";

const { t } = useI18n();
const toast = useToast();
const { status, probe, activate, deactivate, setExportDir } = usePDFActivation();

const dialogOpen = ref(false);
const candidates = ref<ChromeCandidate[]>([]);
const probing = ref(false);

const isActive = computed(() => status.value?.active === true);

const sourceLabel = computed(() => {
  switch (status.value?.source) {
    case "system":
      return t("pdf.status.source.system");
    case "managed":
      return t("pdf.status.source.managed");
    default:
      return t("pdf.status.source.unset");
  }
});

const activatedLabel = computed(() => {
  const a = status.value?.activated_at;
  if (!a) return "";
  const d = a instanceof Date ? a : new Date(String(a));
  return Number.isNaN(d.getTime()) ? String(a) : d.toLocaleString();
});

async function openProbeDialog() {
  dialogOpen.value = true;
  await doProbe();
}

async function doProbe() {
  probing.value = true;
  const r = await probe();
  probing.value = false;
  if (r.ok) {
    candidates.value = r.result.candidates ?? [];
  } else {
    candidates.value = [];
    toast.error("pdf.toast.probe_failed", [r.message]);
  }
}

async function pickCandidate(c: ChromeCandidate) {
  const r = await activate({ browser_bin: c.path });
  if (r.ok) {
    dialogOpen.value = false;
    toast.success("pdf.toast.activated");
  } else {
    toast.error("pdf.toast.activate_failed", [r.message]);
  }
}

async function doDeactivate() {
  const r = await deactivate();
  if (r.ok) {
    toast.success("pdf.toast.deactivated");
  } else {
    toast.error("pdf.toast.deactivate_failed", [r.message]);
  }
}

async function doChangeExportDir() {
  let picked = "";
  try {
    picked = await DialogSvc.ChooseDirectory();
  } catch {
    return; // user cancelled or picker error — treat as no-op
  }
  if (!picked) return;
  const r = await setExportDir(picked);
  if (r.ok) {
    toast.success("pdf.toast.export_dir_set");
  } else {
    toast.error("pdf.toast.export_dir_failed", [r.message]);
  }
}

async function doClearExportDir() {
  const r = await setExportDir("");
  if (r.ok) {
    toast.success("pdf.toast.export_dir_cleared");
  } else {
    toast.error("pdf.toast.export_dir_failed", [r.message]);
  }
}

// Picoloom frontmatter directive reference — fetched as a per-locale
// markdown doc from the backend (go:embed) and rendered to HTML via
// the existing render service. Lazy-loaded on first expand of the
// <details> block so the panel stays cheap until the user asks.
import { Service as PdfSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/pdf";
import { Service as RenderSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/render";

const directivesHTML = ref("");
const directivesLoaded = ref(false);
const directivesLoading = ref(false);

const { locale } = useI18n();

async function loadDirectives() {
  if (directivesLoaded.value || directivesLoading.value) return;
  directivesLoading.value = true;
  try {
    const md = await PdfSvc.GetDirectivesDoc(locale.value);
    directivesHTML.value = await RenderSvc.RenderHTML(md);
    directivesLoaded.value = true;
  } catch {
    // Silently degrade — the <summary> still renders, body empty.
    directivesHTML.value = "";
  } finally {
    directivesLoading.value = false;
  }
}
</script>

<template>
  <p class="section-info">{{ t('pdf.info') }}</p>

  <div class="pdf-status-row">
    <span
      class="pdf-status-pill"
      :class="isActive ? 'active' : 'inactive'"
    >{{ isActive ? t('pdf.status.active') : t('pdf.status.inactive') }}</span>
  </div>

  <dl v-if="isActive" class="pdf-detail-grid">
    <dt>{{ t('pdf.field.browser') }}</dt>
    <dd class="pdf-detail-path">{{ status?.browser_bin }}</dd>
    <dt>{{ t('pdf.field.source') }}</dt>
    <dd>{{ sourceLabel }}</dd>
    <dt v-if="status?.version">{{ t('pdf.field.version') }}</dt>
    <dd v-if="status?.version">{{ status.version }}</dd>
    <dt v-if="activatedLabel">{{ t('pdf.field.activated_at') }}</dt>
    <dd v-if="activatedLabel">{{ activatedLabel }}</dd>
  </dl>

  <div class="pdf-action-row">
    <button
      v-if="!isActive"
      class="tool-btn primary"
      @click="openProbeDialog"
    >{{ t('pdf.action.activate') }}</button>
    <template v-else>
      <button class="tool-btn" @click="doDeactivate">{{ t('pdf.action.deactivate') }}</button>
      <button class="tool-btn" @click="openProbeDialog">{{ t('pdf.action.reconfigure') }}</button>
    </template>
  </div>

  <dl class="pdf-detail-grid">
    <dt>{{ t('pdf.field.export_dir') }}</dt>
    <dd>
      <span v-if="status?.export_dir" class="pdf-detail-path">{{ status.export_dir }}</span>
      <span v-else class="pdf-detail-muted">{{ t('pdf.field.export_dir.unset') }}</span>
    </dd>
  </dl>

  <div class="pdf-action-row">
    <button class="tool-btn" @click="doChangeExportDir">{{ t('pdf.action.change_export_dir') }}</button>
    <button
      v-if="status?.export_dir"
      class="tool-btn"
      @click="doClearExportDir"
    >{{ t('pdf.action.clear_export_dir') }}</button>
  </div>

  <details class="pdf-directives" @toggle="loadDirectives">
    <summary>{{ t('pdf.directives.title') }}</summary>
    <p v-if="directivesLoading" class="muted small">{{ t('pdf.directives.loading') }}</p>
    <div v-else class="pdf-directives-body markdown-body" v-html="directivesHTML"></div>
  </details>

  <Modal
    :open="dialogOpen"
    :title="t('pdf.dialog.title')"
    width="640px"
    @close="dialogOpen = false"
  >
    <p class="muted small">{{ t('pdf.dialog.intro') }}</p>

    <p v-if="probing" class="muted small">{{ t('pdf.dialog.probing') }}</p>

    <ul v-else-if="candidates.length > 0" class="pdf-candidate-list">
      <li v-for="c in candidates" :key="c.path" class="pdf-candidate-row">
        <div class="pdf-candidate-meta">
          <div class="pdf-candidate-path">{{ c.path }}</div>
          <div class="pdf-candidate-sub">
            <span class="pdf-candidate-source">{{ c.source === 'system' ? t('pdf.status.source.system') : t('pdf.status.source.managed') }}</span>
            <span v-if="c.version" class="pdf-candidate-version">{{ c.version }}</span>
          </div>
        </div>
        <button class="tool-btn primary" @click="pickCandidate(c)">{{ t('pdf.dialog.use') }}</button>
      </li>
    </ul>

    <p v-else class="muted">{{ t('pdf.dialog.no_candidates') }}</p>

    <template #footer>
      <button class="tool-btn" type="button" :disabled="probing" @click="doProbe">{{ t('pdf.dialog.refresh') }}</button>
      <button class="tool-btn" type="button" @click="dialogOpen = false">{{ t('pdf.dialog.cancel') }}</button>
    </template>
  </Modal>
</template>
