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
  label: string;
  icon: string;
  component: Component;
}

export const WORKSPACES: WorkspaceDef[] = [
  { id: "templates", label: "Templates", icon: "T", component: TemplatesWorkspace },
  { id: "storage",   label: "Storage",   icon: "S", component: StorageWorkspace },
  { id: "profiles",  label: "Profiles",  icon: "P", component: ProfilesWorkspace },
  { id: "settings",  label: "Settings",  icon: "⚙", component: SettingsWorkspace },
  { id: "plugins",   label: "Plugins",   icon: "◧", component: PluginsWorkspace },
  { id: "about",     label: "About",     icon: "i", component: AboutWorkspace },
];
