import { Service as NavSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/nav";
import { useToast } from "./useToast";

// useFormidableLink - thin Vue wrapper around the Go nav module.
//
// The backend owns parsing, validation, config persistence, and emits
// `nav:changed`. App.vue's global listener reloads config and flips
// the active workspace; this composable only handles the "fire and
// forget the click" part plus toasting on failure.

export function useFormidableLink() {
  const toast = useToast();

  async function follow(href: string): Promise<boolean> {
    if (!href.startsWith("formidable://")) return false;
    const result = await NavSvc.NavigateToFormidable(href);
    if (!result?.success) {
      toast.error("status.template.load.failed", [result?.error ?? href]);
    }
    return true; // handled - don't fall through to the browser default
  }

  return { follow };
}
