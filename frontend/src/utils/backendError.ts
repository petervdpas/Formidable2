// Wails marshals Go errors into a JSON envelope of the shape
// `{message, cause, kind}` and throws them as a JS Error whose
// `.message` is that JSON string. Calling `String(err)` therefore
// yields "Error: {json}" - fine for debugging, terrible for toasts.
//
// backendErrMessage unwraps the envelope and returns the inner Go
// error message ("git: pull: worktree contains unstaged changes")
// when present, falling back to the raw `.message` or String(err)
// otherwise. Use it everywhere a backend error reaches a toast or
// inline status banner.

export function backendErrMessage(err: unknown): string {
  if (!(err instanceof Error)) return String(err);
  return unwrapMessage(err.message);
}

// Some Go errors are themselves JSON (connection.InvokeError marshals
// {code, message, status} out of Error()), so Wails wraps an envelope in an
// envelope and one parse leaves a JSON blob in the toast. Peel while each
// layer is JSON carrying a string `message`; the bound stops a pathological
// payload from looping.
const MAX_ENVELOPES = 4;

function unwrapMessage(raw: string): string {
  let out = raw;
  for (let i = 0; i < MAX_ENVELOPES; i++) {
    let parsed: unknown;
    try {
      parsed = JSON.parse(out);
    } catch {
      return out; // not JSON: this layer is the human message.
    }
    const inner = (parsed as { message?: unknown } | null)?.message;
    if (typeof inner !== "string" || inner === "") return out;
    out = inner;
  }
  return out;
}

// Typed envelope produced by pdf.ExportError on the Go side. The Go
// Error() returns a JSON string; Wails wraps that string into its own
// {message, cause, kind} envelope. Two parses unwrap both layers.
export interface ExportErrorEnvelope {
  code: string;
  message: string;
  hint?: string;
}

export function exportErrorOf(err: unknown): ExportErrorEnvelope | null {
  if (!(err instanceof Error)) return null;
  let inner = err.message;
  try {
    const wails = JSON.parse(err.message);
    if (wails && typeof wails.message === "string") inner = wails.message;
  } catch {
    // err.message wasn't the Wails envelope - try as ExportError directly.
  }
  try {
    const parsed = JSON.parse(inner);
    if (parsed && typeof parsed.code === "string") {
      return {
        code: parsed.code,
        message: typeof parsed.message === "string" ? parsed.message : "",
        hint: typeof parsed.hint === "string" ? parsed.hint : undefined,
      };
    }
  } catch {
    // not a JSON-shaped ExportError
  }
  return null;
}
