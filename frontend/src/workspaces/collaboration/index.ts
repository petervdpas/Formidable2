import type { Component } from "vue";
import CollaborationGit from "./CollaborationGit.vue";
import CollaborationGigot from "./CollaborationGigot.vue";

export type CollaborationSectionId = "current-service";
export type CollaborationBackend = "git" | "gigot";

export interface CollaborationSection {
  id: CollaborationSectionId;
  labelKey: string;
}

// Sidebar rows. For now there's a single "Current Service" row whose
// main pane content adapts to the active remote backend (read from
// config.remote_backend). When real Git-/GiGot-specific operations
// land they'll come in as additional sections, possibly tagged with
// a backend filter so the sidebar stays scoped to the active service.
export const COLLABORATION_SECTIONS: CollaborationSection[] = [
  { id: "current-service", labelKey: "workspace.collaboration.section.current_service" },
];

// Per-backend view components. The workspace picks one of these to
// render based on config.remote_backend. "none" is handled as an
// empty-main fallback at the workspace level (defensive — the
// ribbon ghosts + App.vue redirects when backend === "none").
export const COLLABORATION_BACKEND_VIEWS: Record<CollaborationBackend, Component> = {
  git: CollaborationGit,
  gigot: CollaborationGigot,
};
