<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import GitStatus from "../../components/collaboration/GitStatus.vue";
import AuthorIdentityDialog from "../../components/collaboration/AuthorIdentityDialog.vue";
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
    errorMsg.value = String(err);
    status.value = null;
    if (announce) toast.error("workspace.collaboration.status.error", [String(err)]);
  } finally {
    if (my === reqId) loading.value = false;
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
    toast.error("workspace.collaboration.fetch.error", [String(err)]);
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
    toast.error("workspace.collaboration.push.error", [String(err)]);
  } finally {
    pushing.value = false;
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
    toast.error("workspace.collaboration.commit.error", [String(err)]);
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
    toast.error("workspace.collaboration.status.discard_error", [file, String(err)]);
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
    @refresh="load(true)"
    @fetch="fetchRemote"
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
  </FormSection>

  <div v-if="status && !status.clean && !status.detached" class="git-commit-actions">
    <button
      type="button"
      class="tool-btn primary"
      :disabled="!canCommit"
      @click="commit"
    >
      {{ inFlight ? t('workspace.collaboration.commit.running') : t('workspace.collaboration.commit.button') }}
    </button>
  </div>

  <div
    v-if="status && !status.detached && status.tracking"
    class="git-push-actions"
  >
    <span v-if="status.ahead > 0" class="git-push-summary">
      {{ t('workspace.collaboration.push.ahead_summary', [status.ahead]) }}
    </span>
    <button
      type="button"
      class="tool-btn primary"
      :disabled="!canPush"
      @click="push"
    >
      <i class="fa-solid fa-cloud-arrow-up" aria-hidden="true"></i>
      {{ pushing ? t('workspace.collaboration.push.running') : t('workspace.collaboration.push.button') }}
    </button>
  </div>

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
