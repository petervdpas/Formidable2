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
  if (err instanceof Error) {
    try {
      const parsed = JSON.parse(err.message);
      if (parsed && typeof parsed.message === "string") {
        return parsed.message;
      }
    } catch {
      // err.message wasn't JSON - fall through to the raw message.
    }
    return err.message;
  }
  return String(err);
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
