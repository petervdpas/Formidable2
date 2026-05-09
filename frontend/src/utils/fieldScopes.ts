import type { Field } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

export function recomputeLevelScopes(fields: Field[]): void {
  let scope = 0;
  for (const f of fields) {
    switch ((f.type || "").toLowerCase()) {
      case "loopstart":
        f.level_scope = scope;
        scope++;
        break;
      case "loopstop":
        scope = Math.max(0, scope - 1);
        f.level_scope = scope;
        break;
      default:
        f.level_scope = scope;
    }
  }
}
