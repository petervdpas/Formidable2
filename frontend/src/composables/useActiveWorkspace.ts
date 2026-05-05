import { ref } from "vue";
import type { WorkspaceId } from "../workspaces";

// Module-scope singleton so any component can navigate without
// prop-drilling. App.vue is the only writer for the *initial* value;
// other components write via setActive() (e.g. "Edit in Settings"
// jumping from the Profiles workspace).
const active = ref<WorkspaceId>("templates");

export function useActiveWorkspace() {
  return {
    active,
    setActive: (id: WorkspaceId) => { active.value = id; },
  };
}
