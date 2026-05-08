import type { Component } from "vue";
import CurrentService from "./CurrentService.vue";

export type CollaborationSectionId = "current-service";

export interface CollaborationSection {
  id: CollaborationSectionId;
  labelKey: string;
  component: Component;
}

// Sidebar rows. For now there's a single "Current Service" row whose
// main pane content adapts to the active remote backend (read from
// config.remote_backend) inside CurrentService.vue. When real Git-/
// GiGot-specific operations land they'll come in as additional
// sections, optionally tagged with a backend filter so the sidebar
// stays scoped to the active service.
export const COLLABORATION_SECTIONS: CollaborationSection[] = [
  {
    id: "current-service",
    labelKey: "workspace.collaboration.section.current_service",
    component: CurrentService,
  },
];
