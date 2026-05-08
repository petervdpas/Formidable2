<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import GitStatus from "../../components/collaboration/GitStatus.vue";
import ConfirmDialog from "../../components/ConfirmDialog.vue";
import {
  FormSection,
  FormRow,
  TextareaField,
} from "../../components/fields";
import { Service as GitSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/git";
import { Service as SystemSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/system";
import { useConfig } from "../../composables/useConfig";
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
  modified: string[];
  untracked: string[];
  staged: string[];
  deleted: string[];
  renamed: string[];
  conflicted: string[];
};

const { t } = useI18n();
const { config } = useConfig();
const toast = useToast();

const gitRoot = computed(() => config.value?.git_root ?? "");
const cfg = computed(() => config.value);

const status = ref<Status | null>(null);
const loading = ref(false);
const notARepo = ref(false);
const errorMsg = ref<string>("");

const message = ref("");
const inFlight = ref(false);

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
const canCommit = computed(() => {
  if (inFlight.value) return false;
  if (!status.value) return false;
  if (status.value.clean) return false;
  if (status.value.detached) return false;
  return message.value.trim() !== "";
});

async function commit() {
  if (!canCommit.value) return;
  const c = cfg.value;
  if (!c) return;
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
