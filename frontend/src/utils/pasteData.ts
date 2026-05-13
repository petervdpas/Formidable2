// Parses clipboard text from spreadsheet apps (Excel, Numbers, Google
// Sheets) into a 2D string array. Mirrors utils/pasteDataUtils.js from
// the original Formidable.

export type ParseResult = {
  rows: string[][];
  separator: "\t" | "," | ";";
};

export function parsePastedRows(text: string): ParseResult {
  const lines = (text ?? "")
    .replace(/\r\n/g, "\n")
    .replace(/\r/g, "\n")
    .split("\n");
  while (lines.length > 0 && lines[lines.length - 1] === "") {
    lines.pop();
  }
  if (lines.length === 0) {
    return { rows: [], separator: "\t" };
  }
  const separator = detectSeparator(lines);
  const rows = lines.map((line) =>
    separator === "\t"
      ? line.split("\t")
      : line.split(separator).map((cell) => cell.trim()),
  );
  return { rows, separator };
}

// First column of each row → flat list of values (drop trailing empties
// per row but keep blanks the user intentionally left between values).
export function rowsToListValues(rows: string[][]): string[] {
  return rows.map((r) => (r.length > 0 ? r[0] : "")).filter((v) => v !== "");
}

function detectSeparator(lines: string[]): "\t" | "," | ";" {
  const tabs = countAll(lines, "\t");
  if (tabs > 0) return "\t";
  const semis = countAll(lines, ";");
  const commas = countAll(lines, ",");
  return semis > commas ? ";" : ",";
}

function countAll(lines: string[], ch: string): number {
  let n = 0;
  for (const line of lines) {
    for (let i = 0; i < line.length; i++) {
      if (line[i] === ch) n++;
    }
  }
  return n;
}
