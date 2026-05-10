import type { Component } from "vue";
import type { Config } from "../../composables/useConfig";
import InformationAbout from "./InformationAbout.vue";
import InformationInternalServer from "./InformationInternalServer.vue";
import InformationMonitoring from "./InformationMonitoring.vue";
import InformationJournalFeed from "./InformationJournalFeed.vue";
import InformationLogging from "./InformationLogging.vue";

export type InformationCategoryId =
  | "about"
  | "internal-server"
  | "monitoring"
  | "journal-feed"
  | "logging";

export interface InformationCategory {
  id: InformationCategoryId;
  labelKey: string;
  component: Component;
  /** Optional predicate; entry is hidden when this returns false. */
  available?: (cfg: Config | null) => boolean;
}

export const INFORMATION_CATEGORIES: InformationCategory[] = [
  { id: "about",           labelKey: "workspace.information.section.about",           component: InformationAbout },
  { id: "internal-server", labelKey: "workspace.information.section.internal_server", component: InformationInternalServer },
  { id: "monitoring",      labelKey: "workspace.information.section.monitoring",      component: InformationMonitoring },
  { id: "journal-feed",    labelKey: "workspace.information.section.journal_feed",    component: InformationJournalFeed },
  {
    id: "logging",
    labelKey: "workspace.information.section.logging",
    component: InformationLogging,
    available: (cfg) => !!cfg?.logging_enabled,
  },
];
