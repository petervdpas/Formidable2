<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import CodeEditor from "./CodeEditor.vue";
import Tabs from "./Tabs.vue";
import {
  FormSection,
  FormRow,
  FormSwitchRow,
  TextField,
  SelectField,
} from "./fields";
import { Service as PdfSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/pdf";
import {
  InjectConfig,
  InjectCoverConfig,
  InjectPageConfig,
  InjectTOCConfig,
  InjectFooterConfig,
  InjectSignatureConfig,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/pdf/models";
import type {
  CoverDescriptor,
  ThemeDescriptor,
  PageSizeDescriptor,
  OrientationDescriptor,
  FooterPositionDescriptor,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/pdf/models";
import { backendErrMessage } from "../utils/backendError";
import { useToast } from "../composables/useToast";

// Wizard for building a picoloom v2 PDF frontmatter scaffold. The
// user toggles per-block switches and fills dropdowns/text inputs;
// the backend's BuildFrontmatter renders the YAML deterministically.
// All option lists (themes, covers, page sizes, orientations, footer
// positions) come from the backend per the backend-owns-data rule.

const props = defineProps<{
  open: boolean;
  /** Template name - used as the default Cover.Title pre-fill. */
  templateName?: string;
}>();
const emit = defineEmits<{
  (e: "cancel"): void;
  (e: "apply", scaffold: string): void;
}>();

const { t } = useI18n();
const toast = useToast();

// Backend-driven option lists.
const themes = ref<ThemeDescriptor[]>([]);
const covers = ref<CoverDescriptor[]>([]);
const pageSizes = ref<PageSizeDescriptor[]>([]);
const orientations = ref<OrientationDescriptor[]>([]);
const footerPositions = ref<FooterPositionDescriptor[]>([]);

// Per-block enable toggles. Cover + Page on by default (most common
// starting state); the rest off.
const coverOn = ref(true);
const pageOn = ref(true);
const tocOn = ref(false);
const footerOn = ref(false);
const signatureOn = ref(false);

// Style is always visible (no block to gate).
const style = ref("");

// Cover fields.
const coverTemplate = ref("classic");
const coverTitle = ref("");
const coverSubtitle = ref("");
const coverAuthor = ref("");
const coverOrganization = ref("");
const coverDate = ref("");
const coverVersion = ref("");
const coverLogo = ref("");

// Top-level keywords. Comma-separated in the UI; split into a string
// array on the way to the backend so it round-trips as a real YAML
// sequence (and lands in the PDF Info dict as /Keywords after the
// pdfcpu post-process pass).
const keywordsInput = ref("");

// Page fields.
const pageSize = ref("a4");
const pageOrientation = ref("portrait");
const pageMargin = ref(1.0);

// TOC fields.
const tocTitle = ref("Contents");
const tocMinDepth = ref(1);
const tocMaxDepth = ref(3);

// Footer fields.
const footerPosition = ref("center");
const footerShowPageNumber = ref(true);
const footerText = ref("");
const footerDate = ref("");
const footerStatus = ref("");
const footerDocumentID = ref("");

// Signature fields.
const sigName = ref("");
const sigTitle = ref("");
const sigEmail = ref("");
const sigOrganization = ref("");
const sigImagePath = ref("");
const sigPhone = ref("");
const sigAddress = ref("");
const sigDepartment = ref("");

// Live preview output. Re-renders on every change via the watcher
// below - cheap (pure-function backend call, no I/O).
const previewYAML = ref("");
const previewError = ref("");
const previewOpen = ref(false);

type TabId = "page" | "cover" | "toc" | "footer" | "signature";
const activeTab = ref<TabId>("page");
const tabItems = computed(() => [
  { id: "page", label: t("pdf.inject.tabs.page") },
  { id: "cover", label: t("pdf.inject.tabs.cover") },
  { id: "toc", label: t("pdf.inject.tabs.toc") },
  { id: "footer", label: t("pdf.inject.tabs.footer") },
  { id: "signature", label: t("pdf.inject.tabs.signature") },
]);

// Pre-fill cover title from the template name when the dialog opens.
// Doesn't clobber an existing value (user might re-open after typing).
watch(
  () => props.open,
  async (isOpen) => {
    if (!isOpen) return;
    previewError.value = "";
    previewOpen.value = false;
    activeTab.value = "page";
    if (!coverTitle.value && props.templateName) {
      coverTitle.value = props.templateName;
    }
    try {
      const [t1, t2, t3, t4, t5] = await Promise.all([
        PdfSvc.ListThemes(),
        PdfSvc.ListCovers(),
        PdfSvc.ListPageSizes(),
        PdfSvc.ListPageOrientations(),
        PdfSvc.ListFooterPositions(),
      ]);
      themes.value = t1 ?? [];
      covers.value = (t2 ?? []).filter((c) => c.ok);
      pageSizes.value = t3 ?? [];
      orientations.value = t4 ?? [];
      footerPositions.value = t5 ?? [];
    } catch (e) {
      toast.error("workspace.templates.pdf_fm.inject_failed", [backendErrMessage(e)]);
    }
  },
);

const themeOptions = computed(() => [
  { value: "", label: t("pdf.export.dialog.theme.default_picoloom") },
  ...themes.value.map((th) => ({
    value: th.name,
    label: themeLabel(th.name),
  })),
]);
const THEME_LABEL_KEYS: Record<string, string> = {
  academic: "pdf.export.dialog.theme.academic",
  corporate: "pdf.export.dialog.theme.corporate",
  creative: "pdf.export.dialog.theme.creative",
  invoice: "pdf.export.dialog.theme.invoice",
  legal: "pdf.export.dialog.theme.legal",
  manuscript: "pdf.export.dialog.theme.manuscript",
  technical: "pdf.export.dialog.theme.technical",
};
function themeLabel(name: string): string {
  const key = THEME_LABEL_KEYS[name];
  if (!key) return name;
  const translated = t(key);
  return translated === key ? name : translated;
}

const coverOptions = computed(() =>
  covers.value.map((c) => ({ value: c.name, label: c.label || c.name })),
);

const pageSizeOptions = computed(() =>
  pageSizes.value.map((s) => ({ value: s.name, label: pageSizeLabel(s.name) })),
);
const PAGE_SIZE_LABEL_KEYS: Record<string, string> = {
  a4: "pdf.inject.page_size.a4",
  letter: "pdf.inject.page_size.letter",
  legal: "pdf.inject.page_size.legal",
};
function pageSizeLabel(name: string): string {
  const key = PAGE_SIZE_LABEL_KEYS[name];
  if (!key) return name;
  const translated = t(key);
  return translated === key ? name : translated;
}

const orientationOptions = computed(() =>
  orientations.value.map((o) => ({ value: o.name, label: orientationLabel(o.name) })),
);
const ORIENTATION_LABEL_KEYS: Record<string, string> = {
  portrait: "pdf.inject.orientation.portrait",
  landscape: "pdf.inject.orientation.landscape",
};
function orientationLabel(name: string): string {
  const key = ORIENTATION_LABEL_KEYS[name];
  if (!key) return name;
  const translated = t(key);
  return translated === key ? name : translated;
}

const footerPositionOptions = computed(() =>
  footerPositions.value.map((p) => ({ value: p.name, label: footerPositionLabel(p.name) })),
);
const FOOTER_POSITION_LABEL_KEYS: Record<string, string> = {
  left: "pdf.inject.footer_position.left",
  center: "pdf.inject.footer_position.center",
  right: "pdf.inject.footer_position.right",
};
function footerPositionLabel(name: string): string {
  const key = FOOTER_POSITION_LABEL_KEYS[name];
  if (!key) return name;
  const translated = t(key);
  return translated === key ? name : translated;
}

// Build the InjectConfig payload from the current ref state. The
// `enabled: true` flag is implicit on the backend - a non-nil block
// pointer means "include this block".
//
// Wails-generated classes accept Partial<T> in their constructors so
// we can omit fields rather than spelling out every undefined.
function buildConfig(): InjectConfig {
  const cfg = new InjectConfig({ style: style.value });
  const kws = keywordsInput.value
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
  if (kws.length > 0) {
    cfg.keywords = kws;
  }
  if (coverOn.value) {
    cfg.cover = new InjectCoverConfig({
      template: coverTemplate.value,
      title: coverTitle.value,
      subtitle: coverSubtitle.value,
      author: coverAuthor.value,
      organization: coverOrganization.value,
      date: coverDate.value,
      version: coverVersion.value,
      logo: coverLogo.value,
    });
  }
  if (pageOn.value) {
    cfg.page = new InjectPageConfig({
      size: pageSize.value,
      orientation: pageOrientation.value,
      margin: pageMargin.value,
    });
  }
  if (tocOn.value) {
    cfg.toc = new InjectTOCConfig({
      title: tocTitle.value,
      min_depth: tocMinDepth.value,
      max_depth: tocMaxDepth.value,
    });
  }
  if (footerOn.value && footerHasContent.value) {
    cfg.footer = new InjectFooterConfig({
      position: footerPosition.value,
      show_page_number: footerShowPageNumber.value,
      date: footerDate.value,
      status: footerStatus.value,
      text: footerText.value,
      document_id: footerDocumentID.value,
    });
  }
  if (signatureOn.value && signatureHasContent.value) {
    cfg.signature = new InjectSignatureConfig({
      name: sigName.value,
      title: sigTitle.value,
      email: sigEmail.value,
      organization: sigOrganization.value,
      image_path: sigImagePath.value,
      phone: sigPhone.value,
      address: sigAddress.value,
      department: sigDepartment.value,
    });
  }
  return cfg;
}

// A footer is meaningful when at least the page-number gauge is on
// or any of the text-bearing fields is non-empty. With everything
// off + empty, picoloom emits an empty footer container - drop the
// block entirely instead.
const footerHasContent = computed(
  () =>
    footerShowPageNumber.value ||
    !!footerText.value.trim() ||
    !!footerDate.value.trim() ||
    !!footerStatus.value.trim() ||
    !!footerDocumentID.value.trim(),
);

// Same shape for signatures: picoloom's signature.html template
// gates every line on a field being set; with all eight fields
// empty the output is three empty <div>s. Drop the block.
const signatureHasContent = computed(
  () =>
    !!sigName.value.trim() ||
    !!sigTitle.value.trim() ||
    !!sigEmail.value.trim() ||
    !!sigOrganization.value.trim() ||
    !!sigImagePath.value.trim() ||
    !!sigPhone.value.trim() ||
    !!sigAddress.value.trim() ||
    !!sigDepartment.value.trim(),
);

async function refreshPreview() {
  if (!props.open) return;
  previewError.value = "";
  try {
    previewYAML.value = await PdfSvc.BuildFrontmatter(buildConfig());
  } catch (e) {
    previewYAML.value = "";
    previewError.value = backendErrMessage(e);
  }
}

// Re-render preview on any input change. Cheap - pure function on
// the backend, no I/O.
watch(
  [
    () => props.open, style, coverOn, pageOn, tocOn, footerOn, signatureOn,
    coverTemplate, coverTitle, coverSubtitle, coverAuthor, coverOrganization,
    coverDate, coverVersion, coverLogo, keywordsInput,
    pageSize, pageOrientation, pageMargin,
    tocTitle, tocMinDepth, tocMaxDepth,
    footerPosition, footerShowPageNumber, footerText, footerDate, footerStatus, footerDocumentID,
    sigName, sigTitle, sigEmail, sigOrganization, sigImagePath, sigPhone, sigAddress, sigDepartment,
  ],
  () => { void refreshPreview(); },
);

async function onApply() {
  await refreshPreview();
  if (previewError.value) return;
  emit("apply", previewYAML.value);
}
</script>

<template>
  <Modal
    :open="open"
    :title="t('workspace.templates.pdf_fm.title_inject')"
    width="760px"
    @close="emit('cancel')"
  >
    <p class="muted small">
      {{ t('workspace.templates.pdf_fm.intro_inject') }}
    </p>

    <FormSection :title="t('pdf.inject.style.section')">
      <FormRow :label="t('pdf.inject.style.field')">
        <SelectField v-model="style" :options="themeOptions" />
      </FormRow>
    </FormSection>

    <div class="pdf-inject-tabs">
      <Tabs v-model="activeTab" :items="tabItems">
        <template #page>
          <div class="pdf-inject-tab-pane">
            <FormSwitchRow
              v-model="pageOn"
              :label="t('pdf.inject.page.enable')"
              :description="t('pdf.inject.page.help')"
            />
            <template v-if="pageOn">
              <FormRow :label="t('pdf.inject.page.size')">
                <SelectField v-model="pageSize" :options="pageSizeOptions" />
              </FormRow>
              <FormRow :label="t('pdf.inject.page.orientation')">
                <SelectField v-model="pageOrientation" :options="orientationOptions" />
              </FormRow>
              <FormRow :label="t('pdf.inject.page.margin')">
                <input
                  v-model.number="pageMargin"
                  type="number"
                  step="0.1"
                  min="0"
                  class="pdf-inject-number"
                />
              </FormRow>
            </template>
          </div>
        </template>

        <template #cover>
          <div class="pdf-inject-tab-pane">
            <FormSwitchRow
              v-model="coverOn"
              :label="t('pdf.inject.cover.enable')"
              :description="t('pdf.inject.cover.help')"
            />
            <template v-if="coverOn">
              <FormRow :label="t('pdf.inject.cover.template')">
                <SelectField v-model="coverTemplate" :options="coverOptions" />
              </FormRow>
              <FormRow :label="t('pdf.inject.cover.title')">
                <TextField v-model="coverTitle" />
              </FormRow>
              <FormRow :label="t('pdf.inject.cover.subtitle')">
                <TextField v-model="coverSubtitle" />
              </FormRow>
              <FormRow :label="t('pdf.inject.cover.author')">
                <TextField v-model="coverAuthor" />
              </FormRow>
              <FormRow :label="t('pdf.inject.keywords.field')">
                <TextField
                  v-model="keywordsInput"
                  :placeholder="t('pdf.inject.keywords.placeholder')"
                />
              </FormRow>
              <FormRow :label="t('pdf.inject.cover.organization')">
                <TextField v-model="coverOrganization" />
              </FormRow>
              <FormRow :label="t('pdf.inject.cover.date')">
                <TextField v-model="coverDate" placeholder="YYYY-MM-DD" />
              </FormRow>
              <FormRow :label="t('pdf.inject.cover.version')">
                <TextField v-model="coverVersion" />
              </FormRow>
              <FormRow :label="t('pdf.inject.cover.logo')">
                <TextField v-model="coverLogo" :placeholder="t('pdf.inject.cover.logo_placeholder')" />
              </FormRow>
            </template>
          </div>
        </template>

        <template #toc>
          <div class="pdf-inject-tab-pane">
            <FormSwitchRow
              v-model="tocOn"
              :label="t('pdf.inject.toc.enable')"
              :description="t('pdf.inject.toc.help')"
            />
            <template v-if="tocOn">
              <FormRow :label="t('pdf.inject.toc.title')">
                <TextField v-model="tocTitle" />
              </FormRow>
              <FormRow :label="t('pdf.inject.toc.min_depth')">
                <input v-model.number="tocMinDepth" type="number" min="1" max="6" class="pdf-inject-number" />
              </FormRow>
              <FormRow :label="t('pdf.inject.toc.max_depth')">
                <input v-model.number="tocMaxDepth" type="number" min="1" max="6" class="pdf-inject-number" />
              </FormRow>
            </template>
          </div>
        </template>

        <template #footer>
          <div class="pdf-inject-tab-pane">
            <FormSwitchRow
              v-model="footerOn"
              :label="t('pdf.inject.footer.enable')"
              :description="t('pdf.inject.footer.help')"
            />
            <template v-if="footerOn">
              <FormRow :label="t('pdf.inject.footer.position')">
                <SelectField v-model="footerPosition" :options="footerPositionOptions" />
              </FormRow>
              <FormSwitchRow
                v-model="footerShowPageNumber"
                :label="t('pdf.inject.footer.show_page_number')"
              />
              <FormRow :label="t('pdf.inject.footer.text')">
                <TextField v-model="footerText" />
              </FormRow>
              <FormRow :label="t('pdf.inject.footer.date')">
                <TextField v-model="footerDate" />
              </FormRow>
              <FormRow :label="t('pdf.inject.footer.status')">
                <TextField v-model="footerStatus" />
              </FormRow>
              <FormRow :label="t('pdf.inject.footer.document_id')">
                <TextField v-model="footerDocumentID" />
              </FormRow>
              <p v-if="!footerHasContent" class="pdf-inject-empty-warn">
                {{ t('pdf.inject.footer.empty_warn') }}
              </p>
            </template>
          </div>
        </template>

        <template #signature>
          <div class="pdf-inject-tab-pane">
            <FormSwitchRow
              v-model="signatureOn"
              :label="t('pdf.inject.signature.enable')"
              :description="t('pdf.inject.signature.help')"
            />
            <template v-if="signatureOn">
              <FormRow :label="t('pdf.inject.signature.name')">
                <TextField v-model="sigName" />
              </FormRow>
              <FormRow :label="t('pdf.inject.signature.title')">
                <TextField v-model="sigTitle" />
              </FormRow>
              <FormRow :label="t('pdf.inject.signature.email')">
                <TextField v-model="sigEmail" />
              </FormRow>
              <FormRow :label="t('pdf.inject.signature.organization')">
                <TextField v-model="sigOrganization" />
              </FormRow>
              <FormRow :label="t('pdf.inject.signature.image_path')">
                <TextField v-model="sigImagePath" />
              </FormRow>
              <FormRow :label="t('pdf.inject.signature.phone')">
                <TextField v-model="sigPhone" />
              </FormRow>
              <FormRow :label="t('pdf.inject.signature.address')">
                <TextField v-model="sigAddress" />
              </FormRow>
              <FormRow :label="t('pdf.inject.signature.department')">
                <TextField v-model="sigDepartment" />
              </FormRow>
              <p v-if="!signatureHasContent" class="pdf-inject-empty-warn">
                {{ t('pdf.inject.signature.empty_warn') }}
              </p>
            </template>
          </div>
        </template>
      </Tabs>
    </div>

    <details class="pdf-inject-preview" :open="previewOpen" @toggle="(e) => (previewOpen = (e.target as HTMLDetailsElement).open)">
      <summary>{{ t('pdf.inject.preview.title') }}</summary>
      <p v-if="previewError" class="form-error">{{ previewError }}</p>
      <CodeEditor
        v-else
        :model-value="previewYAML"
        lang="markdown"
        :readonly="true"
        :height="240"
        :title="t('pdf.inject.preview.title')"
      />
    </details>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('cancel')">
        {{ t('common.cancel') }}
      </button>
      <button class="tool-btn primary" type="button" @click="onApply">
        {{ t('workspace.templates.pdf_fm.action.apply') }}
      </button>
    </template>
  </Modal>
</template>
