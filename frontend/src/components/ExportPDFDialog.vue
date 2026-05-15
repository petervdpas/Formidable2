<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import { useDialog } from "../composables/useDialog";
import { useToast } from "../composables/useToast";
import { usePDFActivation } from "../composables/usePDFActivation";
import { Service as PdfSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/pdf";
import type {
  CoverDescriptor,
  ThemeDescriptor,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/pdf/models";
import { Service as StorageSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import { Service as SystemSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/system";
import { backendErrMessage, exportErrorOf } from "../utils/backendError";

const knownExportCodes = new Set<string>([
  "engine_inactive",
  "render_failed",
  "cover_logo_missing",
  "cover_template_invalid",
  "signature_image_missing",
  "directive_invalid",
  "style_not_found",
  "browser_unreachable",
  "render_timeout",
  "empty_markdown",
  "html_conversion_failed",
  "pdf_generation_failed",
  "save_failed",
  "unknown",
]);

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
const { status, refreshLastExport } = usePDFActivation();

// Theme list comes from the backend (PdfSvc.ListThemes) — never
// hardcoded here. Names are picoloom's canonical keys; human labels
// come from `pdf.export.dialog.theme.<name>` i18n keys with the raw
// name as the fallback for any theme the backend adds before its i18n
// landed.
const themes = ref<ThemeDescriptor[]>([]);

function themeLabel(name: string): string {
  const key = `pdf.export.dialog.theme.${name}`;
  const translated = t(key);
  return translated === key ? name : translated;
}

// Sentinel values for the explicit "force off" dropdown entries.
// The backend has dedicated bool flags (ExportOpts.DisableTheme /
// DisableCover); the dialog round-trips through these strings so the
// dropdown widget can express the third state.
const NO_THEME_SENTINEL = "__no_theme__";
const NO_COVER_SENTINEL = "__no_cover__";

const folder = ref("");
const filename = ref("");
const style = ref("");
const coverName = ref("");
const covers = ref<CoverDescriptor[]>([]);
const openAfter = ref(true);
const exporting = ref(false);
const exportError = ref("");
const resolvedTheme = ref("");
const resolvedCover = ref("");
const resolvedCoverDisabled = ref(false);

// Default-option label reveals what frontmatter / template manifest
// would actually resolve to, so the user can see at a glance whether
// a theme/cover is supplied or whether picoloom's built-in default
// kicks in. Falls back to a generic label when resolution fails.
const themeDefaultLabel = computed(() =>
  resolvedTheme.value
    ? t("pdf.export.dialog.theme.default_resolved", [resolvedTheme.value])
    : t("pdf.export.dialog.theme.default_picoloom"),
);
const coverDefaultLabel = computed(() => {
  if (resolvedCoverDisabled.value) {
    return t("pdf.export.dialog.cover.default_disabled");
  }
  return resolvedCover.value
    ? t("pdf.export.dialog.cover.default_resolved", [resolvedCover.value])
    : t("pdf.export.dialog.cover.default_picoloom");
});

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
  coverName.value = "";
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
  // Cover + theme dropdowns live on the backend — scan now so any
  // newly-added entry appears without a restart. Failure here keeps
  // the dialog usable (lists stay empty → user gets picoloom default).
  try {
    covers.value = (await PdfSvc.ListCovers()) ?? [];
  } catch {
    covers.value = [];
  }
  try {
    themes.value = (await PdfSvc.ListThemes()) ?? [];
  } catch {
    themes.value = [];
  }
  // Resolve what the dialog's default options will actually apply so
  // the dropdown labels can tell the truth (e.g. "(frontmatter: technical)"
  // vs "(no theme — picoloom built-in)"). A render error here just
  // leaves the labels in the picoloom-built-in state.
  resolvedTheme.value = "";
  resolvedCover.value = "";
  resolvedCoverDisabled.value = false;
  try {
    const resolved = await PdfSvc.ResolveExportDefaults(
      props.templateFilename,
      props.datafile,
    );
    resolvedTheme.value = resolved.theme ?? "";
    resolvedCover.value = resolved.cover_template ?? "";
    resolvedCoverDisabled.value = resolved.cover_disabled ?? false;
  } catch {
    /* resolution failed — labels degrade to the picoloom-default state */
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
    const disableTheme = style.value === NO_THEME_SENTINEL;
    const disableCover = coverName.value === NO_COVER_SENTINEL;
    const result = await PdfSvc.ExportPDF(props.templateFilename, props.datafile, {
      output_path: outputPath,
      style: disableTheme ? "" : style.value,
      cover_template: disableCover ? "" : coverName.value,
      disable_theme: disableTheme,
      disable_cover: disableCover,
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
    const typed = exportErrorOf(e);
    if (typed && knownExportCodes.has(typed.code) && typed.code !== "unknown") {
      const key = `pdf.toast.export.${typed.code}`;
      exportError.value = t(key);
      toast.error(key);
    } else {
      const msg = typed?.message || backendErrMessage(e);
      exportError.value = msg;
      toast.error("pdf.export.dialog.toast.failed", [msg]);
    }
  } finally {
    exporting.value = false;
    void refreshLastExport();
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
          <option value="">{{ themeDefaultLabel }}</option>
          <option :value="NO_THEME_SENTINEL">{{ t('pdf.export.dialog.theme.none') }}</option>
          <option v-for="th in themes" :key="th.name" :value="th.name">
            {{ themeLabel(th.name) }}
          </option>
        </select>
      </div>

      <div class="pdf-export-row">
        <label class="pdf-export-label">{{ t('pdf.export.dialog.field.cover') }}</label>
        <select v-model="coverName" class="pdf-export-input">
          <option value="">{{ coverDefaultLabel }}</option>
          <option :value="NO_COVER_SENTINEL">{{ t('pdf.export.dialog.cover.none') }}</option>
          <option
            v-for="c in covers"
            :key="c.name"
            :value="c.name"
            :disabled="!c.ok"
          >
            {{ c.label || c.name }}{{ c.ok ? '' : ' ⚠' }}
          </option>
        </select>
        <p v-if="coverName" class="pdf-export-hint">
          {{ covers.find(c => c.name === coverName)?.description || '' }}
        </p>
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
