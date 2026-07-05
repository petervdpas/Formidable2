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
import InformationFonts from "./InformationFonts.vue";
import InformationRenderHelpers from "./InformationRenderHelpers.vue";
import InformationFrontmatterDirectives from "./InformationFrontmatterDirectives.vue";
import InformationManualTopic from "./InformationManualTopic.vue";

export type InformationCategoryId = string;

export interface InformationCategory {
  id: InformationCategoryId;
  labelKey: string;
  /** Leaves carry a component. Branches (with `children`) leave this empty. */
  component?: Component;
  /** Props forwarded to the component when this leaf is active. Lets a
   *  single generic component (e.g. InformationManualTopic) back several
   *  leaves that differ only in input. */
  props?: Record<string, unknown>;
  /** When true, the workspace's top-level page heading is suppressed -
   *  the leaf renders its own H1 (typically because the content is
   *  markdown with its own title). */
  ownsHeading?: boolean;
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
  { id: "fonts", labelKey: "workspace.information.section.fonts", component: InformationFonts },
  {
    id: "help",
    labelKey: "workspace.information.section.help",
    children: [
      { id: "shortcuts",              labelKey: "workspace.information.section.shortcuts",              component: InformationShortcuts },
      { id: "render-helpers",         labelKey: "workspace.information.section.render_helpers",         component: InformationRenderHelpers },
      { id: "frontmatter-directives", labelKey: "workspace.information.section.frontmatter_directives", component: InformationFrontmatterDirectives },
    ],
  },
  {
    id: "manual",
    labelKey: "workspace.information.section.manual",
    children: [
      {
        id: "manual-settings",
        labelKey: "workspace.information.section.manual_settings",
        component: InformationManualTopic,
        props: { topic: "settings" },
        ownsHeading: true,
      },
      {
        id: "manual-profiles",
        labelKey: "workspace.information.section.manual_profiles",
        component: InformationManualTopic,
        props: { topic: "profiles" },
        ownsHeading: true,
      },
      {
        id: "manual-templates",
        labelKey: "workspace.information.section.manual_templates",
        component: InformationManualTopic,
        props: { topic: "templates" },
        ownsHeading: true,
      },
      {
        id: "manual-fields",
        labelKey: "workspace.information.section.manual_fields",
        component: InformationManualTopic,
        props: { topic: "fields" },
        ownsHeading: true,
      },
      {
        id: "manual-pdf",
        labelKey: "workspace.information.section.manual_pdf",
        component: InformationManualTopic,
        props: { topic: "pdf" },
        ownsHeading: true,
      },
      {
        id: "manual-plugins",
        labelKey: "workspace.information.section.manual_plugins",
        component: InformationManualTopic,
        props: { topic: "plugins" },
        ownsHeading: true,
      },
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
