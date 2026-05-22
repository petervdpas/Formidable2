import { computed, ref, watch } from "vue";
import { Service as TemplateSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type {
  ItemField,
  Template,
  ValidationError,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { useTemplates } from "./useTemplates";
import { formatError, type FormattedError } from "../utils/templateValidation";
import { recomputeLevelScopes } from "../utils/fieldScopes";

// Module-scope singleton - there's at most one template being edited
// at a time, and several components (workspace, modal, future toolbar)
// read the same draft / dirty state.
const draft = ref<Template | null>(null);
const itemFieldOptions = ref<ItemField[]>([]);

const { selectedFilename, selectedTemplate, refresh } = useTemplates();

function clone(t: Template | null): Template | null {
  if (!t) return null;
  // JSON round-trip is sufficient - Template is a plain shape
  // (the binding constructor accepts a Partial<Template>).
  return JSON.parse(JSON.stringify(t));
}

function deepEqual(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

const dirty = computed<boolean>(() => {
  if (!draft.value || !selectedTemplate.value) return false;
  if (draft.value.needs_resave) return true;
  return !deepEqual(draft.value, selectedTemplate.value);
});

// Reset the draft whenever the user picks a different template, OR the
// underlying cache refreshes (e.g. after Save reloads).
watch(
  selectedTemplate,
  async (t) => {
    draft.value = clone(t);
    if (draft.value?.fields) {
      recomputeLevelScopes(draft.value.fields);
    }
    if (selectedFilename.value) {
      try {
        itemFieldOptions.value = await TemplateSvc.GetItemFields(selectedFilename.value);
      } catch {
        itemFieldOptions.value = [];
      }
    } else {
      itemFieldOptions.value = [];
    }
  },
  { immediate: true },
);

// SaveOutcome is the union the workspace receives after a save attempt.
// "validation" carries the structured errors so the caller can render
// each one as a localized toast via formatError + vue-i18n.
export type SaveOutcome =
  | { ok: true }
  | { ok: false; reason: "no-draft" }
  | { ok: false; reason: "validation"; errors: FormattedError[] }
  | { ok: false; reason: "exception"; message: string };

async function save(): Promise<SaveOutcome> {
  if (!draft.value || !selectedFilename.value) {
    return { ok: false, reason: "no-draft" };
  }
  try {
    // Backend validation is the source of truth - see
    // internal/modules/template/validate.go. We surface the result
    // here before SaveTemplate so a misshapen template never lands
    // on disk just because the editor allowed it.
    const errors: ValidationError[] = await TemplateSvc.ValidateTemplate(draft.value);
    if (errors && errors.length > 0) {
      return {
        ok: false,
        reason: "validation",
        errors: errors.map(formatError),
      };
    }
    await TemplateSvc.SaveTemplate(selectedFilename.value, draft.value);
    await refresh(); // pull canonical version back into cache
    return { ok: true };
  } catch (err) {
    return { ok: false, reason: "exception", message: String(err) };
  }
}

function reset(): void {
  draft.value = clone(selectedTemplate.value);
}

export function useTemplateEditor() {
  return {
    draft,
    dirty,
    itemFieldOptions,
    save,
    reset,
  };
}
