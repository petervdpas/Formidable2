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

// ChartEnvelope is what a plugin returns to ask the host to render a
// chart: a chart-neutral Result plus presentation hints. A plugin's
// command return value carries one (`chart`) or many (`charts`); see
// extractCharts. type overrides the per-kind default component
// (StatChart); title labels the chart in the dialog.
export interface ChartEnvelope {
  type?: string;
  title?: string;
  result: StatResult;
}

// extractCharts pulls chart envelopes out of a plugin command's return
// value. Recognised shapes:
//   { chart:  { type?, title?, result } }      -> one chart
//   { charts: [ { ... }, { ... } ] }           -> many
// Anything else (plain text, other tables) yields [] so the run dialog
// falls through to its normal text/debug rendering untouched.
export function extractCharts(value: unknown): ChartEnvelope[] {
  if (!value || typeof value !== "object") return [];
  const v = value as Record<string, unknown>;
  const raw =
    Array.isArray(v.charts)
      ? v.charts
      : v.chart && typeof v.chart === "object"
        ? [v.chart]
        : [];
  const out: ChartEnvelope[] = [];
  for (const c of raw) {
    if (!c || typeof c !== "object") continue;
    const e = c as Record<string, unknown>;
    const result = e.result;
    if (result && typeof result === "object") {
      out.push({
        type: typeof e.type === "string" ? e.type : undefined,
        title: typeof e.title === "string" ? e.title : undefined,
        result: result as StatResult,
      });
    }
  }
  return out;
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
