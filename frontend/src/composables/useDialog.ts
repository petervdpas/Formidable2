import { Service as DialogSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/dialog";
import { FileFilter } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/dialog";

export type Filter = { displayName: string; pattern: string };

function toBindingFilter(f: Filter): FileFilter {
  return new FileFilter({ displayName: f.displayName, pattern: f.pattern });
}

/**
 * Opens the OS open-file picker. Returns the chosen absolute path, or
 * "" if the user cancelled. Filters narrow the visible files (each
 * becomes a dropdown entry).
 */
export async function chooseFile(filters: Filter[] = []): Promise<string> {
  return DialogSvc.ChooseFile(filters.map(toBindingFilter));
}

/**
 * Opens the OS save-file picker pre-populated with `defaultName`.
 * Returns "" if the user cancelled.
 */
export async function chooseSaveFile(
  defaultName: string,
  filters: Filter[] = [],
): Promise<string> {
  return DialogSvc.ChooseSaveFile(defaultName, filters.map(toBindingFilter));
}

/** Opens the OS folder picker. Returns "" on cancel. */
export async function chooseDirectory(): Promise<string> {
  return DialogSvc.ChooseDirectory();
}

export function useDialog() {
  return { chooseFile, chooseSaveFile, chooseDirectory };
}
