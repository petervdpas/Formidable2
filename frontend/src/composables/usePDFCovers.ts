import { ref, computed } from "vue";
import * as PdfSvc from "../../bindings/github.com/petervdpas/formidable2/internal/modules/pdf/service";
import type {
  CoverDescriptor,
  CoverValidation,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/pdf/models";
import { backendErrMessage } from "../utils/backendError";

// Filename stems that ship as embedded seeds. Used by the UI to flip
// the "Delete" verb to "Reset" — deleting these only removes the file
// on disk; the next boot's scaffoldCovers run rewrites them. Keep this
// list in sync with internal/modules/pdf/covers/*.html.
const SEED_NAMES = new Set(["classic", "banner", "corporate"]);

// Module-scope singletons so the panel survives sidebar navigation
// without dropping its draft. The Information workspace mounts and
// unmounts as the user clicks between sections — keeping these here
// means the user can switch to "Logging" and back without losing
// unsaved changes.
const covers = ref<CoverDescriptor[]>([]);
const selectedName = ref<string>("");
const draftName = ref<string>("");
const draftHTML = ref<string>("");
const baselineHTML = ref<string>("");
const isNew = ref<boolean>(false);
const lastError = ref<string>("");
const validation = ref<CoverValidation | null>(null);
const validating = ref<boolean>(false);
const loading = ref<boolean>(false);
const saving = ref<boolean>(false);

let validateTimer: number | null = null;

async function refresh() {
  loading.value = true;
  try {
    covers.value = (await PdfSvc.ListCovers()) ?? [];
  } catch (err) {
    lastError.value = backendErrMessage(err);
    covers.value = [];
  } finally {
    loading.value = false;
  }
}

async function loadCoverForEdit(name: string) {
  lastError.value = "";
  if (!name) {
    selectedName.value = "";
    draftName.value = "";
    draftHTML.value = "";
    baselineHTML.value = "";
    isNew.value = false;
    validation.value = null;
    return;
  }
  // Loading an existing cover. ListCovers does NOT carry HTML, so we
  // re-read by name via SaveCover's sibling — but pdf module doesn't
  // expose a GetCover yet. Use ValidateCoverHTML's parsing as a
  // round-trip-free reuse: we just fetch the raw cover via a
  // newly-added pass-through? No — simpler: do not refetch; the user
  // edits from baseline. We hold baseline via the LoadCover endpoint.
  loading.value = true;
  try {
    const html = await PdfSvc.LoadCover(name);
    selectedName.value = name;
    draftName.value = name;
    draftHTML.value = html;
    baselineHTML.value = html;
    isNew.value = false;
    void runValidate(html);
  } catch (err) {
    lastError.value = backendErrMessage(err);
  } finally {
    loading.value = false;
  }
}

const STARTER_TEMPLATE = `<!--
  formidable-cover: 1
  name: New cover
  description: Describe this cover.
-->
<section class="cover cover-mycover"><div class="cover-page">
  <style>
    /* Override picoloom's default theme cascade. Keep the element
       order below (logo → context → doctype → title →
       subtitle → description → organization → meta) so the
       parent picoloom flexbox doesn't fight your layout. */
    .cover-mycover.cover { padding: 0 !important; }
    .cover-mycover .cover-page {
      display: block !important;
      text-align: center !important;
      padding: 1.2in 1in !important;
      align-items: initial !important;
    }
    .cover-mycover .cover-page > * { order: 0 !important; }
    .cover-mycover .cover-page p { text-align: center !important; }
    .cover-mycover .cover-title {
      font-size: 36pt !important; font-weight: 700 !important;
      margin: 0 0 0.25in !important; color: #222 !important;
    }
  </style>

  {{if .Logo}}<img src="{{.Logo}}" alt="Logo" class="cover-logo">{{end}}
  <p class="cover-title">{{.Title}}</p>
  {{if .Subtitle}}<p class="cover-subtitle">{{.Subtitle}}</p>{{end}}
</div></section><span data-cover-end></span>
`;

function startNew() {
  selectedName.value = "";
  draftName.value = "";
  draftHTML.value = STARTER_TEMPLATE;
  baselineHTML.value = "";
  isNew.value = true;
  lastError.value = "";
  void runValidate(STARTER_TEMPLATE);
}

async function runValidate(html: string) {
  if (validateTimer !== null) {
    clearTimeout(validateTimer);
    validateTimer = null;
  }
  validating.value = true;
  try {
    validation.value = await PdfSvc.ValidateCoverHTML(html);
  } catch (err) {
    lastError.value = backendErrMessage(err);
    validation.value = null;
  } finally {
    validating.value = false;
  }
}

// debouncedValidate is what the editor's @update calls on every
// keystroke — pure-function backend validation is cheap, but
// rate-limiting at ~250ms still feels noticeably better than firing
// per-character.
function debouncedValidate(html: string) {
  draftHTML.value = html;
  if (validateTimer !== null) {
    clearTimeout(validateTimer);
  }
  validateTimer = window.setTimeout(() => {
    void runValidate(html);
  }, 250);
}

async function save(): Promise<{ ok: true } | { ok: false; message: string }> {
  if (!draftName.value.trim() || !draftHTML.value.trim()) {
    return { ok: false, message: "Name and HTML are required." };
  }
  saving.value = true;
  lastError.value = "";
  try {
    await PdfSvc.SaveCover(draftName.value.trim(), draftHTML.value);
    baselineHTML.value = draftHTML.value;
    selectedName.value = draftName.value.trim();
    isNew.value = false;
    await refresh();
    return { ok: true as const };
  } catch (err) {
    const message = backendErrMessage(err);
    lastError.value = message;
    return { ok: false as const, message };
  } finally {
    saving.value = false;
  }
}

async function remove(name: string): Promise<{ ok: true } | { ok: false; message: string }> {
  lastError.value = "";
  try {
    await PdfSvc.DeleteCover(name);
    if (selectedName.value === name) {
      selectedName.value = "";
      draftName.value = "";
      draftHTML.value = "";
      baselineHTML.value = "";
      isNew.value = false;
      validation.value = null;
    }
    await refresh();
    return { ok: true as const };
  } catch (err) {
    const message = backendErrMessage(err);
    lastError.value = message;
    return { ok: false as const, message };
  }
}

const dirty = computed(() => draftHTML.value !== baselineHTML.value);
const canSave = computed(
  () =>
    !!draftName.value.trim() &&
    !!draftHTML.value.trim() &&
    (isNew.value || dirty.value) &&
    !saving.value &&
    (validation.value?.ok ?? true),
);

export function usePDFCovers() {
  return {
    covers,
    selectedName,
    draftName,
    draftHTML,
    isNew,
    dirty,
    canSave,
    validation,
    validating,
    loading,
    saving,
    lastError,
    refresh,
    loadCoverForEdit,
    startNew,
    debouncedValidate,
    save,
    remove,
    isSeed: (name: string) => SEED_NAMES.has(name),
  };
}
