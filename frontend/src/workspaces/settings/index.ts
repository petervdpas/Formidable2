import type { Component } from "vue";
import SettingsGeneral from "./SettingsGeneral.vue";
import SettingsHistory from "./SettingsHistory.vue";
import SettingsDisplay from "./SettingsDisplay.vue";
import SettingsLocations from "./SettingsLocations.vue";
import SettingsInternalServer from "./SettingsInternalServer.vue";
import SettingsAdvanced from "./SettingsAdvanced.vue";
import SettingsStatusButtons from "./SettingsStatusButtons.vue";

export type SettingsCategoryId =
  | "general"
  | "history"
  | "display"
  | "locations"
  | "internal-server"
  | "advanced"
  | "status-buttons";

export interface SettingsCategory {
  id: SettingsCategoryId;
  label: string;
  component: Component;
}

export const SETTINGS_CATEGORIES: SettingsCategory[] = [
  { id: "general",         label: "General",         component: SettingsGeneral },
  { id: "history",         label: "History",         component: SettingsHistory },
  { id: "display",         label: "Display",         component: SettingsDisplay },
  { id: "locations",       label: "Locations",       component: SettingsLocations },
  { id: "internal-server", label: "Internal Server", component: SettingsInternalServer },
  { id: "advanced",        label: "Advanced",        component: SettingsAdvanced },
  { id: "status-buttons",  label: "Status Buttons",  component: SettingsStatusButtons },
];
