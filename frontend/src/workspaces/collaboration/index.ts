import type { Component } from "vue";
import CollaborationGit from "./CollaborationGit.vue";
import CollaborationGigot from "./CollaborationGigot.vue";

export type CollaborationCategoryId = "git" | "gigot";

export interface CollaborationCategory {
  id: CollaborationCategoryId;
  labelKey: string;
  component: Component;
}

export const COLLABORATION_CATEGORIES: CollaborationCategory[] = [
  { id: "git",   labelKey: "workspace.collaboration.section.git",   component: CollaborationGit },
  { id: "gigot", labelKey: "workspace.collaboration.section.gigot", component: CollaborationGigot },
];
