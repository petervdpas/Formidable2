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

export type InformationCategoryId = string;

export interface InformationCategory {
  id: InformationCategoryId;
  labelKey: string;
  /** Leaves carry a component. Branches (with `children`) leave this empty. */
  component?: Component;
  /** Nested sub-categories; presence makes this entry a branch. */
  children?: InformationCategory[];
  /** Optional predicate; entry is hidden when this returns false. */
  available?: (cfg: Config | null) => boolean;
}

export const INFORMATION_CATEGORIES: InformationCategory[] = [
  { id: "about",           labelKey: "workspace.information.section.about",           component: InformationAbout },
  { id: "internal-server", labelKey: "workspace.information.section.internal_server", component: InformationInternalServer },
  { id: "monitoring",      labelKey: "workspace.information.section.monitoring",      component: InformationMonitoring },
  { id: "journal-feed",    labelKey: "workspace.information.section.journal_feed",    component: InformationJournalFeed },
  {
    id: "pdf",
    labelKey: "workspace.information.section.pdf",
    children: [
      { id: "pdf-export", labelKey: "workspace.information.section.pdf_export", component: InformationPDFExport },
      { id: "pdf-covers", labelKey: "workspace.information.section.pdf_covers", component: InformationPDFCovers },
    ],
  },
  {
    id: "help",
    labelKey: "workspace.information.section.help",
    children: [
      { id: "shortcuts",      labelKey: "workspace.information.section.shortcuts",      component: InformationShortcuts },
      { id: "render-helpers", labelKey: "workspace.information.section.render_helpers", component: InformationRenderHelpers },
    ],
  },
  {
    id: "logging",
    labelKey: "workspace.information.section.logging",
    component: InformationLogging,
    available: (cfg) => !!cfg?.logging_enabled,
  },
];

/** Walk the (possibly nested) category tree, leaf-first, returning the
 *  first node whose id matches. Returns undefined when not found. */
export function findCategory(
  list: InformationCategory[],
  id: string,
): InformationCategory | undefined {
  for (const c of list) {
    if (c.id === id) return c;
    if (c.children) {
      const hit = findCategory(c.children, id);
      if (hit) return hit;
    }
  }
  return undefined;
}

/** Flatten the tree, depth-first, returning leaves only (entries with
 *  a component). Used to bounce to a visible leaf when the active id
 *  becomes unavailable. */
export function flattenLeaves(list: InformationCategory[]): InformationCategory[] {
  const out: InformationCategory[] = [];
  for (const c of list) {
    if (c.children) {
      out.push(...flattenLeaves(c.children));
    } else if (c.component) {
      out.push(c);
    }
  }
  return out;
}
