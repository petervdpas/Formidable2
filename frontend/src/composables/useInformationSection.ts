import { ref } from "vue";
import type { InformationCategoryId } from "../workspaces/information";

// Module-scope active sub-section for the Information workspace.
// Lifted out of InformationWorkspace.vue so cross-workspace deep-
// links can land directly on a specific category (e.g. the Storage
// workspace's "Export as PDF" action navigating to pdf-export when
// the engine is inactive). Pair with useActiveWorkspace().setActive(
// "information") for the full jump.
//
// InformationWorkspace's existing watcher still corrects the value
// if the active section becomes invisible (e.g. user turns off
// logging while sitting on it).
const active = ref<InformationCategoryId>("about");

export function useInformationSection() {
  return {
    active,
    setActive: (id: InformationCategoryId) => { active.value = id; },
  };
}
