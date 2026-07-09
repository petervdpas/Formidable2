import { onBeforeUnmount, onMounted } from "vue";
import { useI18n } from "vue-i18n";
import { useConfig } from "./useConfig";
import { useToast } from "./useToast";
import { useActiveWorkspace } from "./useActiveWorkspace";
import { useCollaborationSection } from "./useCollaborationSection";
import type { CollaborationSectionId } from "../workspaces/collaboration";
import { confirmLeave } from "./useNavGuard";
import {
  Service as GitSvc,
  FetchOptions,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/git";
import { Service as GigotSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/gigot";

// useSyncNudge is the app-shell watcher that fixes "users forget to pull".
// The status-bar pollers only read LOCAL state, so a git repo never notices
// the remote moved. This watcher does a read-only remote check for the active
// backend on startup, every few minutes, and whenever the window regains focus.
// When it finds the local checkout is behind, it raises a sticky toast that
// routes the user to the Collaboration page to pull (git) or sync (gigot). It
// never pulls on its own: the fetch is read-only and the actual pull stays a
// deliberate click on the proper page.
const POLL_MS = 5 * 60 * 1000;

export function useSyncNudge(): void {
  const { config } = useConfig();
  const { t } = useI18n();
  const toast = useToast();
  const { setActive: setWorkspace } = useActiveWorkspace();
  const { setActive: setSection } = useCollaborationSection();

  let timer: ReturnType<typeof setInterval> | undefined;
  let running = false;
  // Edge-trigger: nudge once per "fell behind" event, not every tick. Re-arms
  // when the checkout is caught up again (or no remote is configured).
  let armed = true;

  async function openCollaboration(section: CollaborationSectionId): Promise<void> {
    if (!(await confirmLeave())) return; // honor an unsaved-changes guard
    setWorkspace("collaboration");
    setSection(section);
  }

  function nudge(messageKey: string, args: unknown[], section: CollaborationSectionId): void {
    if (!armed) return;
    armed = false;
    toast.warn(messageKey, args, {
      sticky: true,
      force: true,
      dedupeKey: "sync-nudge",
      action: {
        label: t("sync.nudge.action"),
        run: () => void openCollaboration(section),
      },
    });
  }

  async function check(): Promise<void> {
    if (running) return;
    running = true;
    try {
      const backend = config.value?.remote_backend ?? "none";
      if (backend === "git") {
        const status = await GitSvc.FetchStatus(
          FetchOptions.createFrom({ remote: "origin", pat: "" }),
        );
        // The fetch refreshed the tracking refs; let the status badge catch up.
        window.dispatchEvent(new Event("formidable:git-refreshed"));
        const behind = status?.behind ?? 0;
        if (behind > 0) nudge("sync.nudge.git", [behind], "git-sync");
        else armed = true;
      } else if (backend === "gigot") {
        const [head, summary] = await Promise.all([
          GigotSvc.Head(),
          GigotSvc.LedgerSummary(),
        ]);
        window.dispatchEvent(new Event("formidable:gigot-refreshed"));
        const remote = head?.version ?? "";
        const local = summary?.version ?? "";
        const behind = remote !== "" && local !== "" && remote !== local;
        if (behind) nudge("sync.nudge.gigot", [], "gigot-sync");
        else armed = true;
      } else {
        armed = true; // no remote backend: nothing to be behind on
      }
    } catch {
      // Background check: swallow network/auth errors quietly. A failed fetch
      // just means we cannot tell, so we say nothing rather than nag.
    } finally {
      running = false;
    }
  }

  function onVisible(): void {
    if (document.visibilityState === "visible") void check();
  }

  onMounted(() => {
    void check();
    timer = setInterval(() => void check(), POLL_MS);
    document.addEventListener("visibilitychange", onVisible);
  });

  onBeforeUnmount(() => {
    if (timer) clearInterval(timer);
    document.removeEventListener("visibilitychange", onVisible);
  });
}
