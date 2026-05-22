<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import ConfirmDialog from "../components/ConfirmDialog.vue";
import { useRestartGate } from "../composables/useRestartGate";
import { useConfig } from "../composables/useConfig";
import { useCredentialAccount } from "../composables/useCredentialAccount";
import { useToast } from "../composables/useToast";
import { setTopbarMenu } from "../composables/useTopbarMenu";
import { useWorkspacePluginMenu } from "../composables/useWorkspacePluginMenu";
import { useCollaborationSection } from "../composables/useCollaborationSection";
import { backendErrMessage } from "../utils/backendError";
import { Service as GitSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/git";
import { Service as CredentialSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/credential";
import { Service as SystemSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/system";
import {
  COLLABORATION_SECTIONS,
  type CollaborationBackend,
} from "./collaboration";

const { t } = useI18n();
const { bootConfig } = useRestartGate();
const { config } = useConfig();
const { accountFor } = useCredentialAccount();
const toast = useToast();

const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

// Trust config.remote_backend as-is - the canonical list of valid
// backends lives on the Go side (journal.ListSyncBackends). An unknown
// string just means visibleSections will find no matching panel,
// which renders the same "no backend" empty state we already handle.
const backend = computed<CollaborationBackend | null>(() => {
  const b = config.value?.remote_backend;
  return typeof b === "string" && b.length > 0 ? b : null;
});

const gitRoot = computed(() => config.value?.git_root ?? "");

// Sidebar shows backend-agnostic rows (no `backend` tag) plus rows
// matching the active backend. Switching backend mid-session
// reactively re-filters; the watcher below corrects activeId if
// it points at a now-hidden row.
const visibleSections = computed(() =>
  COLLABORATION_SECTIONS.filter(
    (s) => !s.backend || s.backend === backend.value,
  ),
);

const { active: activeId, setActive: setActiveSection } = useCollaborationSection();
const activeSection = computed(
  () =>
    visibleSections.value.find((s) => s.id === activeId.value) ??
    visibleSections.value[0],
);

watch(visibleSections, (sections) => {
  if (!sections.find((s) => s.id === activeId.value)) {
    setActiveSection(sections[0]?.id ?? "current-service");
  }
});

// Defensive empty-main: ribbon ghosting + App.vue redirect should
// keep "none" out of reach, but render a clear fallback if it ever
// happens (deleted config, race condition, manual nav).
const hasBackend = computed(() => backend.value !== null);

// Topbar menu actions. Both target the active Git repo via
// config.git_root → resolved absolute path → GitSvc.RemoteInfo for
// the origin URL. We compute the absolute path lazily inside the
// handler (instead of memoizing) because git_root can change while
// the workspace is mounted.

async function resolveOriginURL(): Promise<string | null> {
  const root = gitRoot.value.trim();
  if (root === "") return null;
  const abs = (await SystemSvc.ResolveAbsolutePath(root)) || root;
  const info = await GitSvc.RemoteInfo(abs);
  const origin = info?.remotes?.find((r) => r.name === "origin");
  return origin?.urls?.[0] ?? null;
}

async function openRemoteInBrowser() {
  try {
    const url = await resolveOriginURL();
    if (!url) {
      toast.warn("workspace.collaboration.open_remote.no_origin");
      return;
    }
    await SystemSvc.OpenExternal(url);
  } catch (err) {
    toast.error("workspace.collaboration.open_remote.error", [backendErrMessage(err)]);
  }
}

// Forget saved token: keychain-backed Delete, with a confirm dialog
// because it's destructive (next push/pull will 401 until the user
// re-saves via Clone). The account string format mirrors the
// Go-side credential package convention.
const forgetTokenOpen = ref(false);
const forgetTokenURL = ref<string>("");

async function askForgetToken() {
  const url = await resolveOriginURL();
  if (!url) {
    toast.warn("workspace.collaboration.open_remote.no_origin");
    return;
  }
  forgetTokenURL.value = url;
  forgetTokenOpen.value = true;
}

async function confirmForgetToken() {
  const url = forgetTokenURL.value;
  forgetTokenOpen.value = false;
  forgetTokenURL.value = "";
  if (!url) return;
  try {
    const account = accountFor("git", url);
    await CredentialSvc.Delete(account);
    toast.success("workspace.collaboration.forget_token.success");
  } catch (err) {
    toast.error("workspace.collaboration.forget_token.error", [backendErrMessage(err)]);
  }
}

// Reactive menu definition. The two repo actions disable when there's
// no git_root configured - the click handlers would just toast a
// warning, but disabling is friendlier UX.
const { buildMenu: buildPluginsMenu } = useWorkspacePluginMenu("collaboration");
setTopbarMenu(() => {
  const plugins = buildPluginsMenu();
  if (backend.value !== "git") {
    return plugins ? [plugins] : [];
  }
  const noRoot = gitRoot.value.trim() === "";
  return [
    {
      type: "group",
      id: "repo",
      labelKey: "menu.repo",
      items: [
        {
          id: "open-remote",
          labelKey: "workspace.collaboration.menu.open_remote",
          disabled: noRoot,
          onClick: openRemoteInBrowser,
        },
        {
          id: "forget-token",
          labelKey: "workspace.collaboration.menu.forget_token",
          disabled: noRoot,
          onClick: askForgetToken,
        },
      ],
    },
    ...(plugins ? [plugins] : []),
  ];
});
</script>

<template>
  <Teleport defer to="#topbar-content">
    <span class="topbar-spacer"></span>
  </Teleport>

  <SplitPane :initial="sidebarWidth">
    <template #sidebar>
      <h2 class="sidebar-title">{{ t('workspace.collaboration.sidebar_title') }}</h2>
      <ul class="sidebar-list">
        <li
          v-for="s in visibleSections"
          :key="s.id"
          :class="['sidebar-row', { active: s.id === activeId }]"
          @click="setActiveSection(s.id)"
        >
          {{ t(s.labelKey) }}
        </li>
      </ul>
    </template>

    <template #main>
      <p
        v-if="!hasBackend"
        class="workspace-empty"
        v-html="t('workspace.collaboration.no_backend')"
      ></p>
      <template v-else>
        <h1 class="workspace-heading">{{ t(activeSection.labelKey) }}</h1>
        <component :is="activeSection.component" />
      </template>
    </template>
  </SplitPane>

  <ConfirmDialog
    :open="forgetTokenOpen"
    :title="t('workspace.collaboration.forget_token.title')"
    :message="t('workspace.collaboration.forget_token.confirm', [forgetTokenURL])"
    :confirm-label="t('workspace.collaboration.menu.forget_token')"
    :cancel-label="t('common.cancel')"
    variant="danger"
    @cancel="forgetTokenOpen = false"
    @confirm="confirmForgetToken"
  />
</template>
