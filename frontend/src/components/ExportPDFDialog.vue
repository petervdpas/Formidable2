<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import { useDialog } from "../composables/useDialog";
import { useToast } from "../composables/useToast";
import { usePDFActivation } from "../composables/usePDFActivation";
import { Service as PdfSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/pdf";
import { Service as StorageSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import { Service as SystemSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/system";
import { backendErrMessage } from "../utils/backendError";

// The dialog assumes PDF export is already active. The Storage
// workspace's "Export PDF…" menu entry is hidden while inactive, so
// this component never has to render the inactive state — its only
// dependency on the activation flow is reading `status.export_dir`
// as the default output folder.
const props = defineProps<{
  open: boolean;
  templateFilename: string;
  datafile: string;
}>();
const emit = defineEmits<{ (e: "close"): void; (e: "exported", path: string): void }>();

const { t } = useI18n();
const { chooseDirectory } = useDialog();
const toast = useToast();
const { status } = usePDFActivation();

// Picoloom's embedded theme names. "" maps to picoloom's default
// (no WithStyle option passed). Stage 6 will add custom-CSS support.
const themes = [
  { value: "", labelKey: "pdf.export.dialog.theme.default_label" },
  { value: "technical", labelKey: "pdf.export.dialog.theme.technical" },
  { value: "academic", labelKey: "pdf.export.dialog.theme.academic" },
  { value: "corporate", labelKey: "pdf.export.dialog.theme.corporate" },
  { value: "legal", labelKey: "pdf.export.dialog.theme.legal" },
  { value: "invoice", labelKey: "pdf.export.dialog.theme.invoice" },
  { value: "manuscript", labelKey: "pdf.export.dialog.theme.manuscript" },
  { value: "creative", labelKey: "pdf.export.dialog.theme.creative" },
];

const folder = ref("");
const filename = ref("");
const style = ref("");
const openAfter = ref(true);
const exporting = ref(false);
const exportError = ref("");

function pdfBasename(datafile: string): string {
  // Mirror the backend's pdfBasename: strip `.meta.json` then any
  // remaining extension, default "export". Keeps the dialog's
  // suggested name in sync with what Manager.Export would produce.
  let name = datafile;
  if (name.endsWith(".meta.json")) {
    name = name.slice(0, -".meta.json".length);
  } else {
    const dot = name.lastIndexOf(".");
    if (dot > 0) name = name.slice(0, dot);
  }
  return (name || "export") + ".pdf";
}

async function resetForOpen() {
  exportError.value = "";
  exporting.value = false;
  filename.value = pdfBasename(props.datafile);
  style.value = "";
  openAfter.value = true;
  // Default folder: status.export_dir > template storage dir > empty.
  if (status.value?.export_dir) {
    folder.value = status.value.export_dir;
  } else if (props.templateFilename) {
    try {
      folder.value = await StorageSvc.TemplateStorageDir(props.templateFilename);
    } catch {
      folder.value = "";
    }
  } else {
    folder.value = "";
  }
}

watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) void resetForOpen();
  },
);

async function pickFolder() {
  try {
    const picked = await chooseDirectory();
    if (picked) folder.value = picked;
  } catch {
    /* user cancelled / picker error — leave as-is */
  }
}

function joinPath(dir: string, name: string): string {
  if (!dir) return name;
  if (dir.endsWith("/") || dir.endsWith("\\")) return dir + name;
  return dir + "/" + name;
}

const canExport = computed(
  () => !!filename.value.trim() && !exporting.value,
);

async function doExport() {
  if (!canExport.value) return;
  exporting.value = true;
  exportError.value = "";
  try {
    const outputPath = joinPath(folder.value.trim(), filename.value.trim());
    const result = await PdfSvc.ExportPDF(props.templateFilename, props.datafile, {
      output_path: outputPath,
      style: style.value,
    });
    toast.success("pdf.export.dialog.toast.success");
    if (openAfter.value && result.path) {
      // Best-effort hand-off to the OS default PDF viewer. Failure
      // here doesn't roll back the export — the file is on disk, the
      // success toast already fired; just surface a soft warning so
      // the user knows their auto-open preference didn't take.
      try {
        await SystemSvc.OpenExternal(result.path);
      } catch (openErr) {
        toast.warn("pdf.export.dialog.toast.open_failed", [backendErrMessage(openErr)]);
      }
    }
    emit("exported", result.path);
    emit("close");
  } catch (e) {
    const msg = backendErrMessage(e);
    exportError.value = msg;
    toast.error("pdf.export.dialog.toast.failed", [msg]);
  } finally {
    exporting.value = false;
  }
}
</script>

<template>
  <Modal
    :open="open"
    :title="t('pdf.export.dialog.title')"
    width="640px"
    @close="emit('close')"
  >
    <div class="pdf-export-form">
      <dl class="pdf-export-target">
        <dt>{{ t('pdf.export.dialog.field.template') }}</dt>
        <dd><code>{{ templateFilename }}</code></dd>
        <dt>{{ t('pdf.export.dialog.field.entry') }}</dt>
        <dd><code>{{ datafile }}</code></dd>
      </dl>

      <div class="pdf-export-row">
        <label class="pdf-export-label">{{ t('pdf.export.dialog.field.folder') }}</label>
        <div class="pdf-export-path-row">
          <input v-model="folder" type="text" class="pdf-export-input" />
          <button type="button" class="tool-btn" @click="pickFolder">
            {{ t('pdf.export.dialog.action.browse') }}
          </button>
        </div>
      </div>

      <div class="pdf-export-row">
        <label class="pdf-export-label">{{ t('pdf.export.dialog.field.filename') }}</label>
        <input v-model="filename" type="text" class="pdf-export-input" />
      </div>

      <div class="pdf-export-row">
        <label class="pdf-export-label">{{ t('pdf.export.dialog.field.theme') }}</label>
        <select v-model="style" class="pdf-export-input">
          <option v-for="th in themes" :key="th.value" :value="th.value">
            {{ t(th.labelKey) }}
          </option>
        </select>
      </div>

      <label class="pdf-export-checkbox">
        <input type="checkbox" v-model="openAfter" />
        <span>{{ t('pdf.export.dialog.field.open_after') }}</span>
      </label>

      <p v-if="exportError" class="form-error">{{ exportError }}</p>
    </div>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('close')">
        {{ t('common.cancel') }}
      </button>
      <button
        class="tool-btn primary"
        type="button"
        :disabled="!canExport"
        @click="doExport"
      >
        {{ exporting ? t('pdf.export.dialog.action.exporting') : t('pdf.export.dialog.action.export') }}
      </button>
    </template>
  </Modal>
</template>
