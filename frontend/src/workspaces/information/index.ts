import type { Component } from "vue";
import type { Config } from "../../composables/useConfig";
import InformationAbout from "./InformationAbout.vue";
import InformationShortcuts from "./InformationShortcuts.vue";
import InformationInternalServer from "./InformationInternalServer.vue";
import InformationMonitoring from "./InformationMonitoring.vue";
import InformationJournalFeed from "./InformationJournalFeed.vue";
import InformationLogging from "./InformationLogging.vue";
import InformationPDFExport from "./InformationPDFExport.vue";
import InformationPDFCovers from "./InformationPDFCovers.vue";
import InformationRenderHelpers from "./InformationRenderHelpers.vue";

export type InformationCategoryId =
  | "about"
  | "shortcuts"
  | "internal-server"
  | "monitoring"
  | "journal-feed"
  | "logging"
  | "pdf-export"
  | "pdf-covers"
  | "render-helpers";

export interface InformationCategory {
  id: InformationCategoryId;
  labelKey: string;
  component: Component;
  /** Optional predicate; entry is hidden when this returns false. */
  available?: (cfg: Config | null) => boolean;
}

export const INFORMATION_CATEGORIES: InformationCategory[] = [
  { id: "about",           labelKey: "workspace.information.section.about",           component: InformationAbout },
  { id: "shortcuts",       labelKey: "workspace.information.section.shortcuts",       component: InformationShortcuts },
  { id: "internal-server", labelKey: "workspace.information.section.internal_server", component: InformationInternalServer },
  { id: "monitoring",      labelKey: "workspace.information.section.monitoring",      component: InformationMonitoring },
  { id: "journal-feed",    labelKey: "workspace.information.section.journal_feed",    component: InformationJournalFeed },
  { id: "pdf-export",      labelKey: "workspace.information.section.pdf_export",      component: InformationPDFExport },
  { id: "pdf-covers",      labelKey: "workspace.information.section.pdf_covers",      component: InformationPDFCovers },
  { id: "render-helpers",  labelKey: "workspace.information.section.render_helpers",  component: InformationRenderHelpers },
  {
    id: "logging",
    labelKey: "workspace.information.section.logging",
    component: InformationLogging,
    available: (cfg) => !!cfg?.logging_enabled,
  },
];
