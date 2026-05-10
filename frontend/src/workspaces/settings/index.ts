import type { Component } from "vue";
import SettingsGeneral from "./SettingsGeneral.vue";
import SettingsHistory from "./SettingsHistory.vue";
import SettingsDisplay from "./SettingsDisplay.vue";
import SettingsLocations from "./SettingsLocations.vue";
import SettingsInternalServer from "./SettingsInternalServer.vue";
import SettingsAdvanced from "./SettingsAdvanced.vue";

export type SettingsCategoryId =
  | "general"
  | "history"
  | "display"
  | "locations"
  | "internal-server"
  | "advanced";

export interface SettingsCategory {
  id: SettingsCategoryId;
  labelKey: string;
  component: Component;
}

export const SETTINGS_CATEGORIES: SettingsCategory[] = [
  { id: "general",         labelKey: "settings.categories.general",         component: SettingsGeneral },
  { id: "history",         labelKey: "settings.categories.history",         component: SettingsHistory },
  { id: "display",         labelKey: "settings.categories.display",         component: SettingsDisplay },
  { id: "locations",       labelKey: "settings.categories.locations",       component: SettingsLocations },
  { id: "internal-server", labelKey: "settings.categories.internal_server", component: SettingsInternalServer },
  { id: "advanced",        labelKey: "settings.categories.advanced",        component: SettingsAdvanced },
];
