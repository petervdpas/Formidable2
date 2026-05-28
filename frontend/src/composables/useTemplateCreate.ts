import { ref } from "vue";
import { useI18n } from "vue-i18n";
import { isValidTemplateFilename, useTemplates } from "./useTemplates";
import { useToast } from "./useToast";
import { useStatusBar } from "./useStatusBar";

// useTemplateCreate
// Owns the "+ New template" modal: input, error message, and the
// submit pipeline that validates the filename, calls the templates
// store's create(), toasts on success, and surfaces the right error
// key on failure (filename collision vs other backend error).
//
// Module dependencies (toast/statusBar/templates) are pulled directly
// instead of being injected. They're all singleton-scoped composables
// in this codebase, so the wiring stays trivial at the call site.
export function useTemplateCreate() {
  const { t } = useI18n();
  const toast = useToast();
  const statusBar = useStatusBar();
  const { create } = useTemplates();

  const open = ref(false);
  const input = ref("");
  const error = ref("");

  function openCreate() {
    input.value = "";
    error.value = "";
    open.value = true;
  }

  async function submitCreate() {
    const name = input.value.trim();
    if (!isValidTemplateFilename(name)) {
      error.value = t("workspace.templates.create.invalid");
      return;
    }
    const result = await create(name);
    if (!result.ok) {
      error.value = result.code === "exists"
        ? t("workspace.templates.create.exists")
        : t("workspace.templates.create.error", [result.message ?? "?"]);
      return;
    }
    toast.success("workspace.templates.create.success", [name]);
    statusBar.setCreated(name);
    open.value = false;
  }

  return {
    open,
    input,
    error,
    openCreate,
    submitCreate,
  };
}
