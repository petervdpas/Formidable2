import { computed, ref, watch } from "vue";
import { Service as TemplateSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type {
  ItemField,
  Template,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { useTemplates } from "./useTemplates";

// Module-scope singleton — there's at most one template being edited
// at a time, and several components (workspace, modal, future toolbar)
// read the same draft / dirty state.
const draft = ref<Template | null>(null);
const itemFieldOptions = ref<ItemField[]>([]);

const { selectedFilename, selectedTemplate, refresh } = useTemplates();

function clone(t: Template | null): Template | null {
  if (!t) return null;
  // JSON round-trip is sufficient — Template is a plain shape
  // (the binding constructor accepts a Partial<Template>).
  return JSON.parse(JSON.stringify(t));
}

function deepEqual(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

const dirty = computed<boolean>(() => {
  if (!draft.value || !selectedTemplate.value) return false;
  return !deepEqual(draft.value, selectedTemplate.value);
});

// Reset the draft whenever the user picks a different template, OR the
// underlying cache refreshes (e.g. after Save reloads).
watch(
  selectedTemplate,
  async (t) => {
    draft.value = clone(t);
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

async function save(): Promise<{ ok: boolean; message?: string }> {
  if (!draft.value || !selectedFilename.value) {
    return { ok: false, message: "no draft" };
  }
  try {
    await TemplateSvc.SaveTemplate(selectedFilename.value, draft.value);
    await refresh(); // pull canonical version back into cache
    return { ok: true };
  } catch (err) {
    return { ok: false, message: String(err) };
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
