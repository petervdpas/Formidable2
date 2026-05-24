// useNavGuard - module-scope registry for an "is it safe to leave?"
// check. The active editing workspace (today: StorageWorkspace)
// registers a guard while it's mounted; navigation surfaces that can
// pull the user away from it - the left-rail workspace switch and the
// app-close hook - await confirmLeave() first.
//
// The guard resolves true when it's safe to proceed (no unsaved
// changes, or the user chose Save / Discard) and false to abort (the
// user chose Cancel). In-workspace navigation (switching entry or
// template) is handled inside StorageWorkspace directly, since it owns
// both the draft and the dialog.

type LeaveGuard = () => Promise<boolean>;

let guard: LeaveGuard | null = null;

export function setNavGuard(fn: LeaveGuard | null): void {
  guard = fn;
}

export async function confirmLeave(): Promise<boolean> {
  return guard ? guard() : true;
}
