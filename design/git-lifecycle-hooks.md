# Git Lifecycle Hooks - Design

Two follow-up features for the collab pipelines, agreed in design 2026-05-20. **Not yet built.**

See [architecture.md](architecture.md) for module conventions. Builds on the shipped git pipeline (`internal/modules/collaboration/git`, see commit history around 2026-05-09) and gigot module (`internal/modules/collaboration/gigot`, 2026-05-14).

---

## Problem

Right now nothing nudges the user about collab state at the natural beats of an editing session:

- **Start of session** - no check whether the local repo is behind upstream. User can edit happily on stale content for hours.
- **End of session** - no guard against quitting with uncommitted files or unpushed commits. Real risk of forgetting work overnight, or coming back to "where did that paragraph go?" the next morning.

## Features

### 1. Startup pull-check

On app start, for each profile with a remote:

1. Run `Fetch` to refresh ahead/behind. Network failure = silent skip + log entry.
2. Inspect ahead/behind.
3. If `behind > 0` and working tree is clean → silently pull (fast-forward), then dispatch `formidable:context-reloaded` so composables refresh.
4. If `behind > 0` and tree is dirty (or divergent) → surface a non-blocking toast/status hint. No auto-action. Existing divergent guard `AlertDialog` handles the resolution path when the user opts in.

**Opt-in.** New per-profile boolean `pull_on_startup`, defaults `false` until the user trusts it.

### 2. Shutdown guard

Intercept the Wails 3 window close event. Query git + gigot status (fast - uses cached ahead/behind, no live fetch). If either backend reports uncommitted files OR unpushed commits, raise one combined modal:

```
N uncommitted files / M unpushed commits
[ Commit & push ]  [ Discard ]  [ Cancel close ]  [ Quit anyway ]
```

**Always on**, not a setting. `Quit anyway` covers the "I know, just let me leave" case so the guard never becomes a productivity tax.

The modal must cover BOTH backends in one decision - not two prompts. Per [feedback_per_backend_separation], git and gigot code stay separate; only the modal *host* component is shared, with each backend rendering its own status row.

---

## Decisions (settled)

| Topic | Decision |
|---|---|
| Startup pull behaviour | Auto-pull when tree is clean; toast otherwise. Reuses existing divergent guard. |
| Startup opt-in | New per-profile `pull_on_startup` boolean (default false). |
| Shutdown guard | Always on. Combined modal covering git + gigot. `Quit anyway` is mandatory. |
| Status discovery on shutdown | MUST be fast - read cached ahead/behind, no network round-trip. |
| Network down on startup | Silent skip + log. Don't block startup, don't toast. |
| Network down on shutdown | Use cached state. The user can still `Quit anyway` if push would fail. |
| Module placement | Hooks live in the existing git / gigot service layers - no new module. |
| i18n | All strings routed through `internal/modules/i18n` per [feedback_i18n_central]. |

## Order of work

1. **Git shutdown guard** - highest value, full infra exists (status, ahead/behind, commit, push).
2. **Git startup pull-check** - with the new `pull_on_startup` profile setting + Settings tab toggle.
3. **Gigot shutdown branch** - *blocked* on gigot sync (push/pull) shipping first. Until then the shutdown guard checks git only and the gigot row in the modal is omitted.
4. **Gigot startup pull-check** - also blocked on gigot sync.

## Open questions (settle when we get there)

- Does the shutdown guard's `Commit & push` button trigger the author dialog if `git.user.*` is unset, or short-circuit to an error? Lean toward the author dialog (same flow as a manual commit) so first-time push from the guard isn't a dead end.
- For the startup pull-check, do we wait for the pull to finish before unblocking the UI, or run it async after first paint? Async after first paint is friendlier; the `context-reloaded` event handles the refresh anyway.
- Sysgit mode: the system git binary may prompt for credentials. Auto-pull-on-startup should probably *not* trigger an interactive prompt in the background - detect sysgit + no cached credentials and downgrade to toast even on a clean tree.

## Non-goals

- No "auto-commit on shutdown". The user composes commit messages; we never invent one.
- No background polling during the session. Startup and shutdown only - anything more is noise.
- No multi-profile shutdown guard. The guard inspects the *active* profile only.
