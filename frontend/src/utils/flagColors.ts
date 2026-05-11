/*
 * Flag-color palette — shared with the backend's
 * `internal/modules/template.FlagColors` set. Order = the order shown
 * in the FlagDefinitionsModal swatch grid.
 *
 * Keep in sync with backend. The backend rejects any color outside this
 * set during template validation, so anything added here must also land
 * in flag_definitions.go's FlagColors map (and vice versa).
 */
export const FLAG_COLORS = [
  "red", "orange", "amber", "yellow",
  "green", "teal", "blue", "purple",
  "pink", "gray", "cyan", "lime",
  "indigo", "rose", "brown", "slate",
] as const;

export type FlagColor = (typeof FLAG_COLORS)[number];

/** Mirror of backend regex `^[A-Z][A-Z0-9 _-]*$`. */
export const FLAG_LABEL_REGEX = /^[A-Z][A-Z0-9 _-]*$/;

export const MAX_FLAG_DEFINITIONS = 16;
