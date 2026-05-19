/*
 * Facet color palette — shared with the backend's
 * `internal/modules/template.FacetColors` set. Order = the order shown
 * in the FacetEditorModal swatch grid.
 *
 * Keep in sync with backend. The backend rejects any color outside this
 * set during template validation, so anything added here must also land
 * in template/facets.go's FacetColors map (and vice versa).
 */
export const FACET_COLORS = [
  "red", "orange", "amber", "yellow",
  "green", "teal", "blue", "purple",
  "pink", "gray", "cyan", "lime",
  "indigo", "rose", "brown", "slate",
] as const;

export type FacetColor = (typeof FACET_COLORS)[number];

/** Mirror of backend regex `^[a-z][a-z0-9_-]*$` for Facet.Key. */
export const FACET_KEY_REGEX = /^[a-z][a-z0-9_-]*$/;

/** Mirror of backend regex `^[A-Z][A-Z0-9 _-]*$` for FacetOption.Label. */
export const FACET_LABEL_REGEX = /^[A-Z][A-Z0-9 _-]*$/;

export const MAX_FACETS = 5;
export const MAX_OPTIONS_PER_FACET = 16;

/*
 * Facet icon palette — kept in sync with the backend's
 * `internal/modules/template.FacetIcons` set. Curated 16 FontAwesome
 * keys covering common facet semantics (status, completion, sizing,
 * review, priority, etc.). Order = swatch grid order in FacetEditorModal.
 */
export const FACET_ICONS = [
  "fa-flag", "fa-check", "fa-star", "fa-heart",
  "fa-bookmark", "fa-bell", "fa-shirt", "fa-circle-info",
  "fa-triangle-exclamation", "fa-circle-question", "fa-eye", "fa-clock",
  "fa-tag", "fa-bug", "fa-gear", "fa-fire",
] as const;

export type FacetIcon = (typeof FACET_ICONS)[number];
