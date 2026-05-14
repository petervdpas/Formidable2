// Bridge browser console.* calls into the backend's slog pipeline.
// Each wrapped call still hits the original console method (devtools
// keeps working) and additionally fires Logging.LogFromFrontend so
// the line appears in formidable.log and the in-app Live tail.
//
// The wrappers are reentrancy-guarded — if LogFromFrontend's runtime
// itself logs (binding errors, network blips), we still write to the
// original console but skip the backend round-trip to prevent a loop.

import * as LoggingSvc from "../../bindings/github.com/petervdpas/formidable2/internal/modules/logging/service";

type Level = "debug" | "info" | "warn" | "error";

type ConsoleMethod = "debug" | "log" | "info" | "warn" | "error";

const methodLevels: Record<ConsoleMethod, Level> = {
  debug: "debug",
  log: "info",
  info: "info",
  warn: "warn",
  error: "error",
};

let inFlight = false;
let installed = false;

export function installConsoleBridge() {
  if (installed) return;
  installed = true;

  (Object.keys(methodLevels) as ConsoleMethod[]).forEach((m) => {
    const original = console[m].bind(console);
    console[m] = (...args: unknown[]) => {
      original(...args);
      if (inFlight) return;
      inFlight = true;
      try {
        const { msg, fields } = formatArgs(m, args);
        if (!msg) return;
        void LoggingSvc.LogFromFrontend(methodLevels[m], msg, fields).catch(
          () => {
            // swallow — we already logged to devtools; don't loop.
          },
        );
      } finally {
        inFlight = false;
      }
    };
  });
}

function formatArgs(
  method: ConsoleMethod,
  args: unknown[],
): { msg: string; fields: Record<string, unknown> } {
  if (args.length === 0) {
    return { msg: "", fields: {} };
  }
  const head = args[0];
  const tail = args.slice(1);

  const msg = typeof head === "string" ? head : safeStringify(head);
  const fields: Record<string, unknown> = { method: `console.${method}` };
  if (tail.length > 0) {
    fields["args"] = tail.map(safeStringify);
  }
  return { msg, fields };
}

function safeStringify(v: unknown): string {
  if (v === null) return "null";
  if (v === undefined) return "undefined";
  if (typeof v === "string") return v;
  if (typeof v === "number" || typeof v === "boolean" || typeof v === "bigint") {
    return String(v);
  }
  if (v instanceof Error) {
    return v.stack ? `${v.name}: ${v.message}\n${v.stack}` : `${v.name}: ${v.message}`;
  }
  try {
    return JSON.stringify(v, circularReplacer());
  } catch {
    return Object.prototype.toString.call(v);
  }
}

function circularReplacer() {
  const seen = new WeakSet<object>();
  return (_key: string, value: unknown) => {
    if (typeof value === "object" && value !== null) {
      if (seen.has(value as object)) return "[Circular]";
      seen.add(value as object);
    }
    return value;
  };
}
