import type { Component } from "vue";
import TemplatesWorkspace from "./workspaces/TemplatesWorkspace.vue";
import StorageWorkspace from "./workspaces/StorageWorkspace.vue";
import ProfilesWorkspace from "./workspaces/ProfilesWorkspace.vue";
import SettingsWorkspace from "./workspaces/SettingsWorkspace.vue";
import PluginsWorkspace from "./workspaces/PluginsWorkspace.vue";
import CollaborationWorkspace from "./workspaces/CollaborationWorkspace.vue";
import InformationWorkspace from "./workspaces/InformationWorkspace.vue";

export type WorkspaceId =
  | "templates"
  | "storage"
  | "profiles"
  | "settings"
  | "plugins"
  | "collaboration"
  | "information";

export interface WorkspaceDef {
  id: WorkspaceId;
  labelKey: string;
  /** SVG basename in src/assets/icons/. Icon component resolves it
   *  via Vite's import.meta.glob — Flaticon illustrations keep their
   *  built-in colors, no theme tokens are applied. */
  iconName: string;
  component: Component;
}

export const WORKSPACES: WorkspaceDef[] = [
  { id: "storage",   labelKey: "ribbon.storage",   iconName: "database",        component: StorageWorkspace },
  { id: "templates", labelKey: "ribbon.templates", iconName: "design-thinking", component: TemplatesWorkspace },
  { id: "settings",  labelKey: "ribbon.settings",  iconName: "settings",        component: SettingsWorkspace },
  { id: "profiles",  labelKey: "ribbon.profiles",  iconName: "programmer",      component: ProfilesWorkspace },
  { id: "collaboration", labelKey: "ribbon.collaboration", iconName: "collaboration", component: CollaborationWorkspace },
  { id: "plugins",   labelKey: "ribbon.plugins",   iconName: "web-plugin",      component: PluginsWorkspace },
  { id: "information", labelKey: "ribbon.information", iconName: "info",          component: InformationWorkspace },
];
