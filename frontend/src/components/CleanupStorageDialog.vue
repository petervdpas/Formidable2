<script setup lang="ts">
import { ref, watch, computed } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import { backendErrMessage } from "../utils/backendError";
import {
  Service as IntegritySvc,
  IssueKind,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/integrity";
import type { Report } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/integrity";

const props = defineProps<{
  open: boolean;
  templateFilename: string;
  templateLabel?: string;
}>();

const emit = defineEmits<{ (e: "close"): void }>();

const { t } = useI18n();

const loading = ref(false);
const report = ref<Report | null>(null);
const error = ref<string>("");

// Reset everything when the dialog opens with a (potentially different)
// template. Closing leaves the last report in place until next open so
// reopening immediately doesn't flash an empty body before fetch.
watch(
  () => props.open,
  (isOpen) => {
    if (!isOpen) return;
    report.value = null;
    error.value = "";
  },
);

async function analyze() {
  if (!props.templateFilename) return;
  loading.value = true;
  error.value = "";
  try {
    report.value = await IntegritySvc.Analyze(props.templateFilename);
  } catch (e) {
    error.value = backendErrMessage(e);
    report.value = null;
  } finally {
    loading.value = false;
  }
}

const hasReport = computed(() => report.value !== null);
const clean = computed(() => hasReport.value && (report.value?.issue_count ?? 0) === 0);

function issueKindLabel(kind: string): string {
  // Wire constants from the bindings — string equality on stable
  // identifiers so the i18n bundle key matches the backend constant.
  switch (kind) {
    case IssueKind.IssueMissingField:    return t("workspace.cleanup.kind.missing_field");
    case IssueKind.IssueExtraField:      return t("workspace.cleanup.kind.extra_field");
    case IssueKind.IssueTypeMismatch:    return t("workspace.cleanup.kind.type_mismatch");
    case IssueKind.IssueBadDateFormat:   return t("workspace.cleanup.kind.bad_date_format");
    case IssueKind.IssueMetaMissing:     return t("workspace.cleanup.kind.meta_missing");
    case IssueKind.IssueMetaBadFormat:   return t("workspace.cleanup.kind.meta_bad_format");
    case IssueKind.IssueUnreadable:      return t("workspace.cleanup.kind.unreadable");
    default: return kind;
  }
}

function issueKindClass(kind: string): string {
  // Map every kind to a severity tier the CSS file styles.
  // "danger" = the form file itself is broken; "warn" = drift in
  // values; "info" = structural drift that's repairable losslessly.
  switch (kind) {
    case IssueKind.IssueUnreadable:
      return "danger";
    case IssueKind.IssueTypeMismatch:
    case IssueKind.IssueBadDateFormat:
    case IssueKind.IssueMetaBadFormat:
    case IssueKind.IssueMetaMissing:
      return "warn";
    default:
      return "info";
  }
}
</script>

<template>
  <Modal
    :open="open"
    :title="t('workspace.cleanup.title')"
    width="720px"
    @close="emit('close')"
  >
    <p class="cleanup-intro muted small">
      {{ t('workspace.cleanup.description') }}
    </p>

    <div class="cleanup-target">
      <span class="cleanup-target-label">{{ t('workspace.cleanup.target') }}</span>
      <code class="cleanup-target-value">{{ templateLabel || templateFilename }}</code>
    </div>

    <div v-if="error" class="cleanup-error" role="alert">{{ error }}</div>

    <div v-if="hasReport && !error" class="cleanup-summary" :class="{ clean }">
      <strong>
        {{ clean
          ? t('workspace.cleanup.summary_clean', [report?.form_count ?? 0])
          : t('workspace.cleanup.summary_issues', [report?.issue_count ?? 0, report?.form_count ?? 0]) }}
      </strong>
    </div>

    <div v-if="hasReport && !clean && !error" class="cleanup-forms">
      <details
        v-for="fr in report?.forms ?? []"
        :key="fr.filename"
        class="cleanup-form"
        open
      >
        <summary class="cleanup-form-summary">
          <code>{{ fr.filename }}</code>
          <span class="cleanup-form-count badge badge-warn">
            {{ t('workspace.cleanup.issues_count', [fr.issues?.length ?? 0]) }}
          </span>
        </summary>
        <ul class="cleanup-issue-list">
          <li
            v-for="(iss, idx) in fr.issues ?? []"
            :key="idx"
            class="cleanup-issue"
            :class="['cleanup-issue-' + issueKindClass(iss.kind)]"
          >
            <span class="cleanup-issue-kind">{{ issueKindLabel(iss.kind) }}</span>
            <code v-if="iss.path" class="cleanup-issue-path">{{ iss.path }}</code>
            <span v-if="iss.detail" class="cleanup-issue-detail muted small">{{ iss.detail }}</span>
          </li>
        </ul>
      </details>
    </div>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('close')">
        {{ t('common.close') }}
      </button>
      <button
        class="tool-btn primary"
        type="button"
        :disabled="loading || !templateFilename"
        @click="analyze"
      >
        {{ loading
          ? t('workspace.cleanup.analyzing')
          : t('workspace.cleanup.analyze') }}
      </button>
    </template>
  </Modal>
</template>
