// Filename helpers shared by the image field and the shape SVG import.
//
// safeFileStem reduces arbitrary user- or file-supplied text to a filename stem
// (no path, no extension): keep letters/digits/space/dash/underscore, collapse
// runs of separators, trim. The caller re-adds the extension it wants.
export function safeFileStem(raw: string): string {
  const noPath = raw.split(/[\\/]/).pop() ?? "";
  const noExt = noPath.replace(/\.[^.]+$/, "");
  return noExt
    .replace(/[^A-Za-z0-9 _-]+/g, "-")
    .replace(/[-\s]+/g, "-")
    .replace(/^-|-$/g, "")
    .trim();
}

// extensionOf returns the lowercased extension (with the dot) or "".
export function extensionOf(name: string): string {
  const i = name.lastIndexOf(".");
  return i > 0 ? name.slice(i) : "";
}
