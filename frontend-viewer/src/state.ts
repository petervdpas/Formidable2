// Log failures rather than swallowing them. No visible banner: errors surface
// in a terminal run / devtools instead.
export function reportError(e: unknown): void {
  console.error("[viewer]", e);
}
