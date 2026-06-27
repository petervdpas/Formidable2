import { ref } from "vue";
import { useI18n } from "vue-i18n";
import { useTemplates } from "./useTemplates";
import { useToast } from "./useToast";
import { useStatusBar } from "./useStatusBar";

// useTemplateCreate
// Owns the "+ New template" modal state: open flag, error message, and
// the submit pipeline that calls the templates store's create(), toasts
// on success, and surfaces the right error key on failure (filename
// collision vs other backend error). The filename itself is gathered and
// format-validated by the shared EntryNameDialog, which emits it here.
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
  const error = ref("");

  function openCreate() {
    error.value = "";
    open.value = true;
  }

  async function submitCreate(filename: string) {
    error.value = "";
    const result = await create(filename);
    if (!result.ok) {
      error.value = result.code === "exists"
        ? t("workspace.templates.create.exists")
        : t("workspace.templates.create.error", [result.message ?? "?"]);
      return;
    }
    toast.success("workspace.templates.create.success", [filename]);
    statusBar.setCreated(filename);
    open.value = false;
  }

  return {
    open,
    error,
    openCreate,
    submitCreate,
  };
}
