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
const { status, lastExport, refreshLastExport, assetServerAddr, probe, activate, deactivate, setExportDir } = usePDFActivation();

function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return "";
  if (ms < 1000) return `${ms} ms`;
  return `${(ms / 1000).toFixed(1)} s`;
}

function formatBytes(b: number): string {
  if (!Number.isFinite(b) || b <= 0) return "";
  if (b < 1024) return `${b} B`;
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} kB`;
  return `${(b / (1024 * 1024)).toFixed(1)} MB`;
}

function formatAt(at: unknown): string {
  if (!at) return "";
  const d = at instanceof Date ? at : new Date(String(at));
  return Number.isNaN(d.getTime()) ? String(at) : d.toLocaleString();
}

const EXPORT_CODE_KEYS: Record<string, string> = {
  browser_unreachable: "pdf.toast.export.browser_unreachable",
  cover_logo_missing: "pdf.toast.export.cover_logo_missing",
  cover_template_invalid: "pdf.toast.export.cover_template_invalid",
  directive_invalid: "pdf.toast.export.directive_invalid",
  empty_markdown: "pdf.toast.export.empty_markdown",
  engine_inactive: "pdf.toast.export.engine_inactive",
  html_conversion_failed: "pdf.toast.export.html_conversion_failed",
  pdf_generation_failed: "pdf.toast.export.pdf_generation_failed",
  render_failed: "pdf.toast.export.render_failed",
  render_timeout: "pdf.toast.export.render_timeout",
  save_failed: "pdf.toast.export.save_failed",
  signature_image_missing: "pdf.toast.export.signature_image_missing",
  style_not_found: "pdf.toast.export.style_not_found",
  unknown: "pdf.toast.export.unknown",
};
function exportCodeLabel(code: string): string {
  const key = EXPORT_CODE_KEYS[code];
  if (!key) return code;
  const translated = t(key);
  return translated === key ? code : translated;
}

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
    <dt>{{ t('pdf.field.asset_server') }}</dt>
    <dd>
      <span
        class="pdf-asset-pill"
        :class="assetServerAddr ? 'running' : 'stopped'"
      >
        {{ assetServerAddr
          ? t('pdf.field.asset_server.running', [assetServerAddr])
          : t('pdf.field.asset_server.stopped') }}
      </span>
    </dd>
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

  <details class="pdf-doctor" @toggle="refreshLastExport">
    <summary>{{ t('pdf.doctor.title') }}</summary>
    <p v-if="!lastExport?.last_success && !lastExport?.last_failure" class="muted small">
      {{ t('pdf.doctor.empty') }}
    </p>

    <div v-if="lastExport?.last_success" class="pdf-doctor-card pdf-doctor-card-success">
      <div class="pdf-doctor-card-header">
        <span class="pdf-doctor-pill pdf-doctor-pill-success">{{ t('pdf.doctor.last_success') }}</span>
        <span class="pdf-doctor-when">{{ formatAt(lastExport.last_success.at) }}</span>
      </div>
      <dl class="pdf-detail-grid">
        <dt>{{ t('pdf.doctor.field.template') }}</dt>
        <dd><code>{{ lastExport.last_success.template }}</code> / <code>{{ lastExport.last_success.datafile }}</code></dd>
        <dt>{{ t('pdf.doctor.field.output') }}</dt>
        <dd class="pdf-detail-path">{{ lastExport.last_success.path }}</dd>
        <dt>{{ t('pdf.doctor.field.duration') }}</dt>
        <dd>{{ formatDuration(lastExport.last_success.duration_ms) }} <span class="muted">·</span> {{ formatBytes(lastExport.last_success.bytes ?? 0) }}</dd>
        <dt v-if="lastExport.last_success.theme">{{ t('pdf.doctor.field.theme') }}</dt>
        <dd v-if="lastExport.last_success.theme">{{ lastExport.last_success.theme }}</dd>
        <dt v-if="lastExport.last_success.has_cover">{{ t('pdf.doctor.field.cover') }}</dt>
        <dd v-if="lastExport.last_success.has_cover">{{ lastExport.last_success.cover || t('pdf.doctor.cover.default') }}</dd>
      </dl>
    </div>

    <div v-if="lastExport?.last_failure" class="pdf-doctor-card pdf-doctor-card-failure">
      <div class="pdf-doctor-card-header">
        <span class="pdf-doctor-pill pdf-doctor-pill-failure">{{ t('pdf.doctor.last_failure') }}</span>
        <span class="pdf-doctor-when">{{ formatAt(lastExport.last_failure.at) }}</span>
      </div>
      <dl class="pdf-detail-grid">
        <dt>{{ t('pdf.doctor.field.template') }}</dt>
        <dd><code>{{ lastExport.last_failure.template }}</code> / <code>{{ lastExport.last_failure.datafile }}</code></dd>
        <dt>{{ t('pdf.doctor.field.code') }}</dt>
        <dd>{{ exportCodeLabel(lastExport.last_failure.code ?? '') }} <span class="muted">({{ lastExport.last_failure.code }})</span></dd>
        <dt>{{ t('pdf.doctor.field.stage') }}</dt>
        <dd><code>{{ lastExport.last_failure.stage }}</code></dd>
        <dt v-if="lastExport.last_failure.err">{{ t('pdf.doctor.field.error') }}</dt>
        <dd v-if="lastExport.last_failure.err" class="pdf-doctor-err">{{ lastExport.last_failure.err }}</dd>
        <dt>{{ t('pdf.doctor.field.duration') }}</dt>
        <dd>{{ formatDuration(lastExport.last_failure.duration_ms) }}</dd>
      </dl>
    </div>
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
