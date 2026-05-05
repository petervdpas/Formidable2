import type { Component } from "vue";
import TemplatesWorkspace from "./workspaces/TemplatesWorkspace.vue";
import StorageWorkspace from "./workspaces/StorageWorkspace.vue";
import ProfilesWorkspace from "./workspaces/ProfilesWorkspace.vue";
import SettingsWorkspace from "./workspaces/SettingsWorkspace.vue";
import PluginsWorkspace from "./workspaces/PluginsWorkspace.vue";
import AboutWorkspace from "./workspaces/AboutWorkspace.vue";

export type WorkspaceId =
  | "templates"
  | "storage"
  | "profiles"
  | "settings"
  | "plugins"
  | "about";

export interface WorkspaceDef {
  id: WorkspaceId;
  labelKey: string;
  icon: string;
  component: Component;
}

export const WORKSPACES: WorkspaceDef[] = [
  { id: "templates", labelKey: "ribbon.templates", icon: "T", component: TemplatesWorkspace },
  { id: "storage",   labelKey: "ribbon.storage",   icon: "S", component: StorageWorkspace },
  { id: "profiles",  labelKey: "ribbon.profiles",  icon: "P", component: ProfilesWorkspace },
  { id: "settings",  labelKey: "ribbon.settings",  icon: "⚙", component: SettingsWorkspace },
  { id: "plugins",   labelKey: "ribbon.plugins",   icon: "◧", component: PluginsWorkspace },
  { id: "about",     labelKey: "ribbon.about",     icon: "i", component: AboutWorkspace },
];
