<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import GitStatus from "../../components/collaboration/GitStatus.vue";
import AuthorIdentityDialog from "../../components/collaboration/AuthorIdentityDialog.vue";
import AlertDialog from "../../components/AlertDialog.vue";
import ConfirmDialog from "../../components/ConfirmDialog.vue";
import {
  FormSection,
  FormRow,
  TextareaField,
} from "../../components/fields";
import { Service as GitSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/git";
import { Service as SystemSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/system";
import { useConfig } from "../../composables/useConfig";
import { isValidAuthor } from "../../composables/useAuthorValidation";
import { useToast } from "../../composables/useToast";
import { backendErrMessage } from "../../utils/backendError";

// Sync workspace — combined status + commit screen for the active
// Git repository. Owns the status-fetch lifecycle so the commit
// panel can read the same status to gate its button. Push will
// land alongside in the next iteration.

type Status = {
  branch: string;
  tracking: string;
  detached: boolean;
  clean: boolean;
  ahead: number;
  behind: number;
  modified: string[];
  untracked: string[];
  staged: string[];
  deleted: string[];
  renamed: string[];
  conflicted: string[];
};

const { t } = useI18n();
const { config, update } = useConfig();
const toast = useToast();

const gitRoot = computed(() => config.value?.git_root ?? "");
const cfg = computed(() => config.value);

const status = ref<Status | null>(null);
const loading = ref(false);
const notARepo = ref(false);
const errorMsg = ref<string>("");

const message = ref("");
const inFlight = ref(false);
const pushing = ref(false);
const pulling = ref(false);
const fetching = ref(false);

// PAT lookup is server-side: GitSvc.Push / Fetch resolve the stored
// PAT from the OS keychain when we send pat="". The frontend never
// touches the secret — the credential.Service intentionally does not
// expose Get to Wails.

// Race guard for status fetches — only the latest result wins.
let reqId = 0;

async function load(announce: boolean) {
  const path = gitRoot.value.trim();
  if (path === "") {
    status.value = null;
    notARepo.value = false;
    errorMsg.value = "";
    return;
  }
  const my = ++reqId;
  loading.value = true;
  errorMsg.value = "";
  notARepo.value = false;
  try {
    const abs = (await SystemSvc.ResolveAbsolutePath(path)) || path;
    const isRepo = await GitSvc.IsGitRepo(abs);
    if (my !== reqId) return;
    if (!isRepo) {
      status.value = null;
      notARepo.value = true;
      if (announce) toast.warn("workspace.collaboration.status.not_a_repo");
      return;
    }
    const s = await GitSvc.Status(abs);
    if (my !== reqId) return;
    status.value = (s as Status | null) ?? null;
    if (announce) toast.success("workspace.collaboration.status.refreshed");
  } catch (err) {
    if (my !== reqId) return;
    errorMsg.value = backendErrMessage(err);
    status.value = null;
    if (announce) toast.error("workspace.collaboration.status.error", [backendErrMessage(err)]);
  } finally {
    if (my === reqId) {
      loading.value = false;
      // Notify other panels that this Sync page just re-read git
      // status. StatusGitQuick listens so the footer reflects a
      // manual Refresh click without waiting for its own poll.
      window.dispatchEvent(new CustomEvent("formidable:git-refreshed"));
    }
  }
}

onMounted(() => load(false));
watch(gitRoot, () => load(false));

// Commit is enabled when there's something to commit, the message
// is non-empty, and the worktree is on a branch (not detached).
// We do NOT gate on the author identity here: the click handler
// intercepts an invalid identity and opens AuthorIdentityDialog,
// which is a much better UX than silently disabling the button.
const canCommit = computed(() => {
  if (inFlight.value) return false;
  if (!status.value) return false;
  if (status.value.clean) return false;
  if (status.value.detached) return false;
  return message.value.trim() !== "";
});

// Author-identity dialog state. Open on Commit click when the active
// profile's name/email fails the shared validators; on Save we write
// to config and resume the commit automatically.
const authorDialogOpen = ref(false);

// Fetch updates the remote-tracking refs so ahead/behind reflects
// reality. Read-only against the worktree; ahead/behind in the
// status panel re-renders after the follow-up local refresh.
async function fetchRemote() {
  if (fetching.value) return;
  fetching.value = true;
  try {
    const abs = (await SystemSvc.ResolveAbsolutePath(gitRoot.value)) || gitRoot.value;
    const result = await GitSvc.Fetch({ path: abs, remote: "origin", pat: "" });
    if (result?.already_up_to_date) {
      toast.info("workspace.collaboration.fetch.up_to_date");
    } else {
      toast.success("workspace.collaboration.fetch.success");
    }
    await load(false);
  } catch (err) {
    toast.error("workspace.collaboration.fetch.error", [backendErrMessage(err)]);
  } finally {
    fetching.value = false;
  }
}

const canPush = computed(() => {
  if (pushing.value) return false;
  if (!status.value) return false;
  if (status.value.detached) return false;
  if (!status.value.tracking) return false;
  return status.value.ahead > 0;
});

const canPull = computed(() => {
  if (pulling.value) return false;
  if (!status.value) return false;
  if (status.value.detached) return false;
  if (!status.value.tracking) return false;
  return status.value.behind > 0;
});

// Pulling onto a dirty worktree fails inside go-git with a
// "worktree contains unstaged changes" error. We intercept the
// click and offer Stash & pull as the primary recovery — the
// journal's pending set drives which paths are stashed (narrower
// than `git status`, so external dirt is left alone).
const pullDirtyOpen = ref(false);

// Override notification: PullWithStash succeeded but some of the
// user's local changes were dropped because pull's content won (the
// path is non-mergeable, or recmerge hit immutable-meta divergence).
// We surface the post-pull commit author so the user knows who to
// coordinate with offline. Stash dir is always trashed — this list
// is the only signal something was lost.
type OverriddenPath = {
  path: string;
  author: string;
  email: string;
  time: string;
  commit: string;
};
const overrideOpen = ref(false);
const overridePaths = ref<OverriddenPath[]>([]);

// Pulling on divergent history (ahead > 0 AND behind > 0) trips
// go-git's fast-forward-only Pull. Same loud-prompt pattern: warn
// the user up-front instead of letting the backend error bubble.
const pullDivergentOpen = ref(false);

async function push() {
  if (!canPush.value) return;
  pushing.value = true;
  try {
    const abs = (await SystemSvc.ResolveAbsolutePath(gitRoot.value)) || gitRoot.value;
    const result = await GitSvc.Push({ path: abs, remote: "origin", pat: "" });
    if (result?.already_up_to_date) {
      toast.info("workspace.collaboration.push.up_to_date");
    } else {
      toast.success("workspace.collaboration.push.success");
    }
    await load(false);
  } catch (err) {
    toast.error("workspace.collaboration.push.error", [backendErrMessage(err)]);
  } finally {
    pushing.value = false;
  }
}

async function pull() {
  if (!canPull.value) return;
  // Pre-flight: dirty worktree → offer Stash & pull. The dialog's
  // confirm path calls pullWithStash; cancel just closes.
  if (status.value && !status.value.clean) {
    pullDirtyOpen.value = true;
    return;
  }
  // Pre-flight: divergent history → go-git's fast-forward-only Pull
  // would refuse anyway. Surface a clear "needs manual resolution"
  // alert instead of the raw backend error.
  if (status.value && status.value.ahead > 0 && status.value.behind > 0) {
    pullDivergentOpen.value = true;
    return;
  }
  pulling.value = true;
  try {
    const abs = (await SystemSvc.ResolveAbsolutePath(gitRoot.value)) || gitRoot.value;
    const result = await GitSvc.Pull({ path: abs, remote: "origin", pat: "" });
    if (result?.already_up_to_date) {
      toast.info("workspace.collaboration.pull.up_to_date");
    } else {
      toast.success("workspace.collaboration.pull.success");
    }
    await load(false);
  } catch (err) {
    toast.error("workspace.collaboration.pull.error", [backendErrMessage(err)]);
  } finally {
    pulling.value = false;
  }
}

// pullWithStash drives the journal-aware auto-stash flow.
//
// Outcomes (silent except for overrides):
//   - all paths cleanly restored / no pending  → toast "pulled".
//   - some paths auto-merged via recmerge      → toast "pulled and merged N".
//   - some paths overridden (pull won)         → AlertDialog naming the
//     other authors so the user can coordinate offline. Stash dir is
//     always trashed; no manual-recovery path.
async function pullWithStash() {
  pullDirtyOpen.value = false;
  pulling.value = true;
  try {
    const abs = (await SystemSvc.ResolveAbsolutePath(gitRoot.value)) || gitRoot.value;
    const result = await GitSvc.PullWithStash({ path: abs, remote: "origin", pat: "" });

    const overridden = (result?.overridden ?? []) as OverriddenPath[];
    const merged = (result?.auto_merged ?? []) as string[];

    if (overridden.length > 0) {
      overridePaths.value = overridden;
      overrideOpen.value = true;
    } else if (merged.length > 0) {
      toast.success("workspace.collaboration.pull.merge_success", [String(merged.length)]);
    } else if (result?.pull?.already_up_to_date) {
      toast.info("workspace.collaboration.pull.up_to_date");
    } else {
      toast.success("workspace.collaboration.pull.success");
    }
    await load(false);
  } catch (err) {
    toast.error("workspace.collaboration.pull.error", [backendErrMessage(err)]);
  } finally {
    pulling.value = false;
  }
}

async function commit() {
  if (!canCommit.value) return;
  const c = cfg.value;
  if (!c) return;

  // Intercept invalid identity: open the dialog instead. The save
  // handler writes to config and re-invokes commit() so the user's
  // click feels like one continuous action.
  if (!isValidAuthor(c.author_name, c.author_email)) {
    authorDialogOpen.value = true;
    return;
  }

  inFlight.value = true;
  try {
    const abs = (await SystemSvc.ResolveAbsolutePath(gitRoot.value)) || gitRoot.value;
    const result = await GitSvc.Commit({
      path: abs,
      message: message.value.trim(),
      author: c.author_name,
      email: c.author_email,
    });
    if (result?.short) {
      toast.success("workspace.collaboration.commit.success", [result.short]);
      message.value = "";
      await load(false);
    }
  } catch (err) {
    toast.error("workspace.collaboration.commit.error", [backendErrMessage(err)]);
  } finally {
    inFlight.value = false;
  }
}

async function saveAuthorAndCommit(name: string, email: string) {
  await update({ author_name: name, author_email: email });
  authorDialogOpen.value = false;
  await commit();
}

// Discard flow: GitStatus emits the file path → we open a confirm
// dialog → on confirm we hit the backend. The backend handles the
// per-status semantics (modified → restore from HEAD; untracked →
// remove; staged-add → unstage + remove). After the call we refresh
// status so the file disappears from its bucket.
const discardOpen = ref(false);
const discardFile = ref("");

function askDiscard(file: string) {
  discardFile.value = file;
  discardOpen.value = true;
}

async function confirmDiscard() {
  const file = discardFile.value;
  discardOpen.value = false;
  if (!file) return;
  try {
    const abs = (await SystemSvc.ResolveAbsolutePath(gitRoot.value)) || gitRoot.value;
    await GitSvc.Discard({ path: abs, file });
    toast.success("workspace.collaboration.status.discarded", [file]);
    await load(false);
  } catch (err) {
    toast.error("workspace.collaboration.status.discard_error", [file, backendErrMessage(err)]);
  } finally {
    discardFile.value = "";
  }
}
</script>

<template>
  <p class="section-info">{{ t('workspace.collaboration.sync.info') }}</p>

  <GitStatus
    :status="status"
    :loading="loading"
    :not-a-repo="notARepo"
    :error-msg="errorMsg"
    :can-pull="canPull"
    :can-push="canPush"
    :pulling="pulling"
    :pushing="pushing"
    @refresh="load(true)"
    @fetch="fetchRemote"
    @pull="pull"
    @push="push"
    @discard="askDiscard"
  />

  <FormSection
    v-if="status && !status.clean && !status.detached"
    :title="t('workspace.collaboration.commit.title')"
  >
    <FormRow :label="t('workspace.collaboration.commit.message')">
      <TextareaField
        v-model="message"
        :placeholder="t('workspace.collaboration.commit.message_placeholder')"
        :rows="3"
      />
    </FormRow>
    <FormRow>
      <div class="git-commit-actions">
        <button
          type="button"
          class="tool-btn primary"
          :disabled="!canCommit"
          @click="commit"
        >
          {{ inFlight ? t('workspace.collaboration.commit.running') : t('workspace.collaboration.commit.button') }}
        </button>
      </div>
    </FormRow>
  </FormSection>

  <ConfirmDialog
    :open="pullDirtyOpen"
    :title="t('workspace.collaboration.pull.dirty_title')"
    :message="t('workspace.collaboration.pull.dirty_message')"
    :confirm-label="t('workspace.collaboration.pull.stash_button')"
    :cancel-label="t('common.cancel')"
    @cancel="pullDirtyOpen = false"
    @confirm="pullWithStash"
  />

  <AlertDialog
    :open="overrideOpen"
    :title="t('workspace.collaboration.pull.override_title')"
    :message="t('workspace.collaboration.pull.override_message', [overridePaths.map(p => `${p.path} (${p.author || 'unknown'}${p.email ? ` <${p.email}>` : ''})`).join('; ')])"
    @close="overrideOpen = false"
  />

  <AlertDialog
    :open="pullDivergentOpen"
    :title="t('workspace.collaboration.pull.divergent_title')"
    :message="t('workspace.collaboration.pull.divergent_message')"
    @close="pullDivergentOpen = false"
  />

  <AuthorIdentityDialog
    :open="authorDialogOpen"
    :initial-name="cfg?.author_name ?? ''"
    :initial-email="cfg?.author_email ?? ''"
    @cancel="authorDialogOpen = false"
    @save="saveAuthorAndCommit"
  />

  <ConfirmDialog
    :open="discardOpen"
    :title="t('workspace.collaboration.status.discard_title')"
    :message="t('workspace.collaboration.status.discard_confirm', [discardFile])"
    :confirm-label="t('workspace.collaboration.status.discard')"
    :cancel-label="t('common.cancel')"
    variant="danger"
    @cancel="discardOpen = false"
    @confirm="confirmDiscard"
  />
</template>
