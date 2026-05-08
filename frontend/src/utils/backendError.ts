// Wails marshals Go errors into a JSON envelope of the shape
// `{message, cause, kind}` and throws them as a JS Error whose
// `.message` is that JSON string. Calling `String(err)` therefore
// yields "Error: {json}" — fine for debugging, terrible for toasts.
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
      // err.message wasn't JSON — fall through to the raw message.
    }
    return err.message;
  }
  return String(err);
}
