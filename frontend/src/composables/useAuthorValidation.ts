// Single source of truth for "is this author identity good enough
// to stamp on a commit?" Used by Settings → General (proactive
// field-level validation) and Repository Sync (commit-time gate).
//
// Two layers:
//   1. Light RFC-ish format check on the email — catches "peter",
//      "peter@@x", trailing whitespace, etc. We deliberately don't
//      pull a full RFC 5322 implementation; it's overkill for an
//      author identity field that gets stamped into git commits.
//   2. Tiny blacklist for the seeded defaults shipped in
//      defaults.go ("unknown" / "*@example.com"). Anything more
//      (rejecting gmail, outlook, role addresses) would be picky
//      and annoying.

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export function isValidAuthorName(name: string | null | undefined): boolean {
  const v = (name ?? "").trim();
  if (v === "") return false;
  if (v.toLowerCase() === "unknown") return false;
  return true;
}

export function isValidAuthorEmail(email: string | null | undefined): boolean {
  const v = (email ?? "").trim();
  if (v === "") return false;
  if (!EMAIL_RE.test(v)) return false;
  if (/@example\.com$/i.test(v)) return false;
  return true;
}

export function isValidAuthor(
  name: string | null | undefined,
  email: string | null | undefined,
): boolean {
  return isValidAuthorName(name) && isValidAuthorEmail(email);
}
