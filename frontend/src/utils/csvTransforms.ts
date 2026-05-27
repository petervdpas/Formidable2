// Shared transform metadata for the CSV import + export dialogs and their
// row components. Kept here (not duplicated per dialog) so the rule set,
// labels, and param hints stay in one place.

export const transformRules: string[] = [
  "none", "trim", "lowercase", "uppercase", "capitalize",
  "trim+lower", "trim+upper", "trim+cap",
  "first-n", "last-n", "split", "bool-match", "split-table",
];

export const transformLabelKey: Record<string, string> = {
  "none": "csv.transform.none",
  "trim": "csv.transform.trim",
  "lowercase": "csv.transform.lowercase",
  "uppercase": "csv.transform.uppercase",
  "capitalize": "csv.transform.capitalize",
  "trim+lower": "csv.transform.trimlower",
  "trim+upper": "csv.transform.trimupper",
  "trim+cap": "csv.transform.trimcap",
  "first-n": "csv.transform.firstn",
  "last-n": "csv.transform.lastn",
  "split": "csv.transform.split",
  "bool-match": "csv.transform.boolmatch",
  "split-table": "csv.transform.splittable",
};

export const paramPlaceholder: Record<string, string> = {
  "first-n": "N",
  "last-n": "N",
  "split": ", ; |",
  "bool-match": "",
  "split-table": "; ,",
};

export const paramInputType: Record<string, "number" | "text"> = {
  "first-n": "number",
  "last-n": "number",
};
