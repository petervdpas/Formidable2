// Derives the set of queryable sources for a template: the same universe
// the stat builder offers (fields flagged use_in_statistics + their table
// columns + facets), shaped for the query backend. The query module reads
// the index's form_values store, which only materializes these, so the
// query column / filter pickers must offer exactly this set.
//
// Unlike the stat builder's SourceRef (which carries a table column *key*
// and lets the backend resolve it), the query backend's Source carries the
// positional form_values.col index directly. That index is the column's
// position in the table field's `options` array - the same contract the
// indexer uses (internal/modules/index/values.go). We resolve it here.

import type { Template } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// QuerySource mirrors the query.Source backend shape (kind/key/col).
export interface QuerySource {
  kind: string; // "field" | "facet"
  key: string;
  col?: number | null;
}

export interface QueryableSource {
  id: string; // stable <select> value: kind|key|col
  label: string;
  source: QuerySource;
  numeric: boolean;
  date: boolean;
  text: boolean; // high-cardinality free text - caller may prefill a limit
  // multi marks a source that fans one row per entry when projected: a
  // table column, or a list/tags/multioption field (stored one-row-per-
  // entry in form_values). Two such columns can't be row-aligned from the
  // index, so the query UI allows at most one per query.
  multi: boolean;
  // Closed value set (dropdown/radio option values, or facet option labels):
  // a filter on this source offers a dropdown instead of free text so the
  // user can't type a value that exists nowhere in the data.
  choices?: { value: string; label: string }[];
}

function srcId(s: QuerySource): string {
  return `${s.kind}|${s.key}|${s.col ?? ""}`;
}

// deriveQueryableSources projects a template's fields + facets into the
// selectable source list. Returns [] for a null template.
export function deriveQueryableSources(tpl: Template | null): QueryableSource[] {
  if (!tpl) return [];
  const out: QueryableSource[] = [];

  for (const f of tpl.fields ?? []) {
    if (!f.use_in_statistics) continue;
    const flabel = f.label || f.key;

    if (f.type === "table") {
      const cols = (f.statistics_columns ?? []) as string[];
      const opts = (f.options ?? []) as Array<Record<string, unknown>>;
      for (const colKey of cols) {
        const idx = opts.findIndex((x) => String(x?.value ?? "") === colKey);
        if (idx < 0) continue; // a stat column with no matching option: skip
        const o = opts[idx];
        const ctype = String(o?.type ?? "string");
        const clabel = String(o?.label ?? colKey);
        const source: QuerySource = { kind: "field", key: f.key, col: idx };
        out.push({
          id: srcId(source),
          label: `${flabel} / ${clabel}`,
          source,
          numeric: ctype === "number",
          date: ctype === "date",
          text: false,
          multi: true, // a table column fans one row per table row
        });
      }
      continue;
    }

    const source: QuerySource = { kind: "field", key: f.key };
    let choices: { value: string; label: string }[] | undefined;
    if (f.type === "dropdown" || f.type === "radio") {
      choices = ((f.options ?? []) as Array<Record<string, unknown>>)
        .map((o) => ({ value: String(o?.value ?? ""), label: String(o?.label ?? o?.value ?? "") }))
        .filter((c) => c.value !== "");
    }
    out.push({
      id: srcId(source),
      label: flabel,
      source,
      numeric: f.type === "number" || f.type === "range",
      date: f.type === "date",
      text: f.type === "text",
      multi: f.type === "list" || f.type === "multioption" || f.type === "tags",
      choices,
    });
  }

  for (const fc of tpl.facets ?? []) {
    const source: QuerySource = { kind: "facet", key: fc.key };
    const choices = (fc.options ?? [])
      .map((o) => o.label)
      .filter((l) => l !== "")
      .map((l) => ({ value: l, label: l }));
    out.push({ id: srcId(source), label: fc.key, source, numeric: false, date: false, text: false, multi: false, choices });
  }

  return out;
}
