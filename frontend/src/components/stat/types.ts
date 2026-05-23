// Chart-neutral statistics shape, mirroring the Go stat.Result
// (internal/modules/stat/stat.go). Kept as a local structural type so
// the SVG chart components don't hard-depend on the generated Stat
// bindings; the generated type is structurally assignable to this, so
// callers can pass either. A plugin returns this same shape (via
// formidable.stats.* / facets.*) inside its chart envelope.

export interface StatSeries {
  name: string;
  values: number[];
}

export interface StatResult {
  /** "distribution" | "crosstab" | "scalar_stats" | "timeseries" */
  kind: string;
  /** x-axis labels; aligned to each series' values by index. */
  categories?: string[];
  /** one or more named numeric rows aligned to categories. */
  series?: StatSeries[];
  /** single-number stats that don't fit the category grid. */
  scalars?: Record<string, number>;
  /** form-count denominator, for percentage display. */
  total: number;
}

/** Stable kind constants, matching the Go side. */
export const StatKind = {
  Distribution: "distribution",
  Crosstab: "crosstab",
  ScalarStats: "scalar_stats",
  TimeSeries: "timeseries",
} as const;

/** Shared chart palette using app theme vars (falls back to literals
 *  so a chart still renders if a var is missing). Mirrors the monitor
 *  module's palette so stats charts look native. */
export const CHART_PALETTE = [
  "var(--color-accent, #5b9cff)",
  "var(--color-accent-2, #f08c5a)",
  "var(--color-success, #5fc37a)",
  "var(--color-warn, #d4a64a)",
  "var(--color-danger, #d96666)",
  "var(--color-text-muted, #888)",
];
