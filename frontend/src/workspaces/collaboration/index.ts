import type { Component } from "vue";
import CurrentService from "./CurrentService.vue";
import GitSync from "./GitSync.vue";
import GitClone from "./GitClone.vue";
import GitCommitGraph from "./GitCommitGraph.vue";
import GigotSync from "./GigotSync.vue";
import GigotCommitGraph from "./GigotCommitGraph.vue";
import GigotClone from "./GigotClone.vue";

export type CollaborationSectionId =
  | "current-service"
  | "git-sync"
  | "git-clone"
  | "git-graph"
  | "gigot-sync"
  | "gigot-graph"
  | "gigot-clone";

// CollaborationBackend is a string keyed by the backend's canonical id
// (see journal.ListSyncBackends on the Go side). Intentionally not a
// union literal type - adding a new backend should NOT require editing
// this file; the only frontend churn is adding the Vue components that
// register against it in COLLABORATION_SECTIONS below.
export type CollaborationBackend = string;

export interface CollaborationSection {
  id: CollaborationSectionId;
  labelKey: string;
  component: Component;
  /** Backend filter - when set, the row is only shown if
   *  config.remote_backend matches. Omit for backend-agnostic rows
   *  like "Current Service". The string must match a backend id known
   *  to the Go side (journal.ListSyncBackends). */
  backend?: CollaborationBackend;
}

// Sidebar rows. "Current Service" is the always-visible overview;
// the rest are operations scoped to a specific backend (the workspace
// filters by config.remote_backend). Order mirrors per backend:
// Current Service → Sync → Commit Graph → Clone. Setup (PAT / bearer)
// is folded into each backend's Clone Repository panel, so there is
// no separate "Connect" sidebar row on either side.
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
    component: GitCommitGraph,
    backend: "git",
  },
  {
    id: "git-clone",
    labelKey: "workspace.collaboration.section.git_clone",
    component: GitClone,
    backend: "git",
  },
  {
    id: "gigot-sync",
    labelKey: "workspace.collaboration.section.gigot_sync",
    component: GigotSync,
    backend: "gigot",
  },
  {
    id: "gigot-graph",
    labelKey: "workspace.collaboration.section.gigot_graph",
    component: GigotCommitGraph,
    backend: "gigot",
  },
  {
    id: "gigot-clone",
    labelKey: "workspace.collaboration.section.gigot_clone",
    component: GigotClone,
    backend: "gigot",
  },
];
