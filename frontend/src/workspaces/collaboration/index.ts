import type { Component } from "vue";
import CurrentService from "./CurrentService.vue";
import GitSync from "./GitSync.vue";
import GitClone from "./GitClone.vue";
import CommitGraphView from "./CommitGraphView.vue";
import GigotConnect from "./GigotConnect.vue";
import GigotClone from "./GigotClone.vue";

export type CollaborationSectionId =
  | "current-service"
  | "git-sync"
  | "git-clone"
  | "git-graph"
  | "gigot-connect"
  | "gigot-clone";
export type CollaborationBackend = "git" | "gigot";

export interface CollaborationSection {
  id: CollaborationSectionId;
  labelKey: string;
  component: Component;
  /** Backend filter — when set, the row is only shown if
   *  config.remote_backend matches. Omit for backend-agnostic rows
   *  like "Current Service". */
  backend?: CollaborationBackend;
}

// Sidebar rows. "Current Service" is the always-visible overview;
// the rest are operations scoped to a specific backend (the workspace
// filters by config.remote_backend). When real GiGot operations land
// they take a backend: "gigot" tag and slot in alongside.
export const COLLABORATION_SECTIONS: CollaborationSection[] = [
  {
    id: "current-service",
    labelKey: "workspace.collaboration.section.current_service",
    component: CurrentService,
  },
  {
    id: "git-sync",
    labelKey: "workspace.collaboration.section.git_sync",
    component: GitSync,
    backend: "git",
  },
  {
    id: "git-graph",
    labelKey: "workspace.collaboration.section.git_graph",
    component: CommitGraphView,
    backend: "git",
  },
  {
    id: "git-clone",
    labelKey: "workspace.collaboration.section.git_clone",
    component: GitClone,
    backend: "git",
  },
  {
    id: "gigot-connect",
    labelKey: "workspace.collaboration.section.gigot_connect",
    component: GigotConnect,
    backend: "gigot",
  },
  {
    id: "gigot-clone",
    labelKey: "workspace.collaboration.section.gigot_clone",
    component: GigotClone,
    backend: "gigot",
  },
];
