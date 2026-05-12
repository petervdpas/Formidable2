import { ref } from "vue";
import type { CollaborationSectionId } from "../workspaces/collaboration";

// Module-scope active sub-section for the Collaboration workspace.
// Lifted out of CollaborationWorkspace.vue so footer status buttons
// (StatusGitQuick today, StatusGigotLoad tomorrow) can deep-link
// into a specific row without prop-drilling or events. Pair with
// useActiveWorkspace().setActive("collaboration") for the full jump.
//
// CollaborationWorkspace's existing watcher still corrects the value
// if the active section becomes invisible (e.g. user flips backend
// from git → none and the git-only sections vanish).
const active = ref<CollaborationSectionId>("current-service");

export function useCollaborationSection() {
  return {
    active,
    setActive: (id: CollaborationSectionId) => { active.value = id; },
  };
}
