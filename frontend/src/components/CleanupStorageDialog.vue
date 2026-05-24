<script setup lang="ts">
import { ref, watch, computed, reactive } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import Badge from "./Badge.vue";
import { backendErrMessage } from "../utils/backendError";
import {
  Service as IntegritySvc,
  IssueKind,
  FixPlan,
  FixPlanItem,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/integrity";
import type {
  Report,
  FixResult,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/integrity";
import { Service as StorageSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import type { MigrateResult } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";

const props = defineProps<{
  open: boolean;
  templateFilename: string;
  templateLabel?: string;
}>();

const emit = defineEmits<{ (e: "close"): void }>();

const { t } = useI18n();

const loading = ref(false);
const repairing = ref(false);
const migrating = ref(false);
const report = ref<Report | null>(null);
const lastFixResult = ref<FixResult | null>(null);
const lastMigrateResult = ref<MigrateResult | null>(null);
const error = ref<string>("");

// Per-kind UI state: which kinds the user wants to repair + the
// strategy picked for each. Resets on every fresh analyze run.
type KindUI = { include: boolean; strategy: string };
const kindUI = reactive<Record<string, KindUI>>({});

// Reset everything when the dialog opens with a (potentially different)
// template.
watch(
  () => props.open,
  (isOpen) => {
    if (!isOpen) return;
    report.value = null;
    lastFixResult.value = null;
    lastMigrateResult.value = null;
    error.value = "";
    for (const k of Object.keys(kindUI)) delete kindUI[k];
  },
);

async function analyze() {
  if (!props.templateFilename) return;
  loading.value = true;
  error.value = "";
  lastFixResult.value = null;
  try {
    report.value = await IntegritySvc.Analyze(props.templateFilename);
    rebuildKindUI();
  } catch (e) {
    error.value = backendErrMessage(e);
    report.value = null;
  } finally {
    loading.value = false;
  }
}

const hasReport = computed(() => report.value !== null);
const clean = computed(() => hasReport.value && (report.value?.issue_count ?? 0) === 0);

// ── per-kind summary ────────────────────────────────────────────────
// Group issues by kind, counting both total issues and how many forms
// have at least one issue of that kind. Reactive on `report`.

type KindSummary = {
  kind: string;
  issueCount: number;
  formCount: number;
  strategies: { id: string; labelKey: string }[];
};

function strategiesFor(kind: string): { id: string; labelKey: string }[] {
  // The order here is the dropdown order; first entry is the default.
  switch (kind) {
    case IssueKind.IssueExtraField:
      return [
        { id: "strip", labelKey: "workspace.cleanup.strategy.strip" },
        { id: "skip",  labelKey: "workspace.cleanup.strategy.skip" },
      ];
    case IssueKind.IssueMissingField:
      return [
        { id: "fill_default", labelKey: "workspace.cleanup.strategy.fill_default" },
        { id: "skip",         labelKey: "workspace.cleanup.strategy.skip" },
      ];
    case IssueKind.IssueTypeMismatch:
    case IssueKind.IssueBadDateFormat:
      return [
        { id: "coerce", labelKey: "workspace.cleanup.strategy.coerce" },
        { id: "clear",  labelKey: "workspace.cleanup.strategy.clear" },
        { id: "skip",   labelKey: "workspace.cleanup.strategy.skip" },
      ];
    case IssueKind.IssueDateAnomaly:
      // No safe automatic conversion - the value didn't fit the column's
      // inferred format. Default to skip (leave it for a manual fix);
      // Clear is the opt-in destructive alternative. Never coerce.
      return [
        { id: "skip",  labelKey: "workspace.cleanup.strategy.skip" },
        { id: "clear", labelKey: "workspace.cleanup.strategy.clear" },
      ];
    case IssueKind.IssueMetaMissing:
      return [
        { id: "mint_uuid", labelKey: "workspace.cleanup.strategy.mint_uuid" },
        { id: "skip",      labelKey: "workspace.cleanup.strategy.skip" },
      ];
    case IssueKind.IssueMetaBadFormat:
      return [
        { id: "restamp", labelKey: "workspace.cleanup.strategy.restamp" },
        { id: "skip",    labelKey: "workspace.cleanup.strategy.skip" },
      ];
    case IssueKind.IssueUnreadable:
      // Unreadable can't be repaired in-app; the only "action" is to
      // open the file in an external editor. The summary still lists
      // it so the user sees the full picture.
      return [
        { id: "skip", labelKey: "workspace.cleanup.strategy.open_external" },
      ];
    default:
      return [{ id: "skip", labelKey: "workspace.cleanup.strategy.skip" }];
  }
}

const summaryRows = computed<KindSummary[]>(() => {
  const r = report.value;
  if (!r || !r.forms) return [];
  const map = new Map<string, { issueCount: number; forms: Set<string> }>();
  for (const fr of r.forms) {
    for (const iss of fr.issues ?? []) {
      const k = iss.kind;
      let entry = map.get(k);
      if (!entry) {
        entry = { issueCount: 0, forms: new Set() };
        map.set(k, entry);
      }
      entry.issueCount++;
      entry.forms.add(fr.filename);
    }
  }
  const rows: KindSummary[] = [];
  for (const [k, e] of map.entries()) {
    rows.push({
      kind: k,
      issueCount: e.issueCount,
      formCount: e.forms.size,
      strategies: strategiesFor(k),
    });
  }
  // Stable display order: severity tier, then kind id.
  rows.sort((a, b) => {
    const ta = issueKindClass(a.kind);
    const tb = issueKindClass(b.kind);
    if (ta !== tb) return ta === "danger" ? -1 : tb === "danger" ? 1 : ta === "warn" ? -1 : 1;
    return a.kind.localeCompare(b.kind);
  });
  return rows;
});

function rebuildKindUI() {
  // After an analyze run, seed each kind with its default strategy
  // and "include = true" so the user can hit Repair Selected without
  // any extra clicks. Unchecking opts a kind out.
  const rows = summaryRows.value;
  for (const r of rows) {
    const def = r.strategies[0]?.id ?? "skip";
    kindUI[r.kind] = {
      include: r.kind !== IssueKind.IssueUnreadable, // unreadable defaults off
      strategy: def,
    };
  }
}

const repairBtnEnabled = computed(() => {
  if (!hasReport.value || clean.value || repairing.value) return false;
  for (const r of summaryRows.value) {
    const ui = kindUI[r.kind];
    if (ui && ui.include && ui.strategy !== "skip") return true;
  }
  return false;
});

async function migrate() {
  if (!props.templateFilename) return;
  migrating.value = true;
  error.value = "";
  try {
    lastMigrateResult.value = await StorageSvc.MigrateTemplateMeta(props.templateFilename);
    // Migration may have fixed meta_bad_format issues lurking in the
    // analyze report; refresh if one was already loaded so the user
    // sees an accurate picture without re-clicking Analyze.
    if (hasReport.value) {
      await analyze();
    }
  } catch (e) {
    error.value = backendErrMessage(e);
  } finally {
    migrating.value = false;
  }
}

async function repair() {
  if (!props.templateFilename) return;
  repairing.value = true;
  error.value = "";
  try {
    const items: FixPlanItem[] = [];
    for (const r of summaryRows.value) {
      const ui = kindUI[r.kind];
      if (!ui || !ui.include) continue;
      items.push(FixPlanItem.createFrom({ kind: r.kind, strategy: ui.strategy }));
    }
    const plan = FixPlan.createFrom({ items });
    lastFixResult.value = await IntegritySvc.Fix(props.templateFilename, plan);
    // Re-fetch a clean analyze; the backend's ScannedAfter count is
    // authoritative but the frontend wants the per-form drill-down too.
    await analyze();
  } catch (e) {
    error.value = backendErrMessage(e);
  } finally {
    repairing.value = false;
  }
}

function issueKindLabel(kind: string): string {
  switch (kind) {
    case IssueKind.IssueMissingField:    return t("workspace.cleanup.kind.missing_field");
    case IssueKind.IssueExtraField:      return t("workspace.cleanup.kind.extra_field");
    case IssueKind.IssueTypeMismatch:    return t("workspace.cleanup.kind.type_mismatch");
    case IssueKind.IssueBadDateFormat:   return t("workspace.cleanup.kind.bad_date_format");
    case IssueKind.IssueDateAnomaly:     return t("workspace.cleanup.kind.date_anomaly");
    case IssueKind.IssueMetaMissing:     return t("workspace.cleanup.kind.meta_missing");
    case IssueKind.IssueMetaBadFormat:   return t("workspace.cleanup.kind.meta_bad_format");
    case IssueKind.IssueUnreadable:      return t("workspace.cleanup.kind.unreadable");
    default: return kind;
  }
}

function issueKindClass(kind: string): string {
  switch (kind) {
    case IssueKind.IssueUnreadable:
      return "danger";
    case IssueKind.IssueTypeMismatch:
    case IssueKind.IssueBadDateFormat:
    case IssueKind.IssueDateAnomaly:
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
    width="780px"
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

    <div v-if="lastFixResult" class="cleanup-fixresult" role="status">
      {{ t('workspace.cleanup.fix_result',
           [lastFixResult.applied, lastFixResult.forms_saved, lastFixResult.scanned_after]) }}
    </div>

    <div v-if="lastMigrateResult" class="cleanup-fixresult" role="status">
      {{ (lastMigrateResult.errors?.length ?? 0) > 0
        ? t('workspace.cleanup.migrate.result_with_errors',
            [lastMigrateResult.migrated, lastMigrateResult.total, lastMigrateResult.skipped, lastMigrateResult.errors?.length ?? 0])
        : t('workspace.cleanup.migrate.result',
            [lastMigrateResult.migrated, lastMigrateResult.total, lastMigrateResult.skipped]) }}
    </div>

    <div v-if="hasReport && !error" class="cleanup-summary" :class="{ clean }">
      <strong>
        {{ clean
          ? t('workspace.cleanup.summary_clean', [report?.form_count ?? 0])
          : t('workspace.cleanup.summary_issues', [report?.issue_count ?? 0, report?.form_count ?? 0]) }}
      </strong>
    </div>

    <!-- By-kind summary table with checkbox + strategy dropdown.
         The repair button consults kindUI[*].include + .strategy when
         building the FixPlan; nothing is written until that button. -->
    <table v-if="hasReport && !clean && !error" class="cleanup-kind-table">
      <thead>
        <tr>
          <th class="cleanup-th-select"></th>
          <th>{{ t('workspace.cleanup.col.kind') }}</th>
          <th class="cleanup-th-count">{{ t('workspace.cleanup.col.issues') }}</th>
          <th class="cleanup-th-count">{{ t('workspace.cleanup.col.forms') }}</th>
          <th>{{ t('workspace.cleanup.col.strategy') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="row in summaryRows"
          :key="row.kind"
          :class="['cleanup-kind-row', 'cleanup-issue-' + issueKindClass(row.kind)]"
        >
          <td class="cleanup-td-select">
            <input
              type="checkbox"
              :checked="kindUI[row.kind]?.include ?? false"
              @change="kindUI[row.kind] = { include: ($event.target as HTMLInputElement).checked, strategy: kindUI[row.kind]?.strategy ?? row.strategies[0].id }"
            />
          </td>
          <td>
            <span class="cleanup-issue-kind">{{ issueKindLabel(row.kind) }}</span>
          </td>
          <td class="cleanup-td-count">{{ row.issueCount }}</td>
          <td class="cleanup-td-count">{{ row.formCount }}</td>
          <td>
            <select
              v-if="row.strategies.length > 1"
              :value="kindUI[row.kind]?.strategy ?? row.strategies[0].id"
              :disabled="!(kindUI[row.kind]?.include ?? false)"
              @change="kindUI[row.kind] = { include: kindUI[row.kind]?.include ?? false, strategy: ($event.target as HTMLSelectElement).value }"
            >
              <option v-for="s in row.strategies" :key="s.id" :value="s.id">
                {{ t(s.labelKey) }}
              </option>
            </select>
            <span v-else class="muted small">{{ t(row.strategies[0].labelKey) }}</span>
          </td>
        </tr>
      </tbody>
    </table>

    <!-- Per-form drill-down (unchanged from phase 1; still useful for
         the "1 issue across 3 forms" cases the summary collapses). -->
    <div v-if="hasReport && !clean && !error" class="cleanup-forms">
      <details
        v-for="fr in report?.forms ?? []"
        :key="fr.filename"
        class="cleanup-form"
      >
        <summary class="cleanup-form-summary">
          <code>{{ fr.filename }}</code>
          <Badge variant="warn" class="cleanup-form-count">
            {{ t('workspace.cleanup.issues_count', [fr.issues?.length ?? 0]) }}
          </Badge>
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
            <code v-if="iss.value" class="cleanup-issue-value">{{ iss.value }}</code>
            <span v-if="iss.suggest" class="cleanup-issue-suggest muted small">&rarr; {{ iss.suggest }}</span>
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
        class="tool-btn"
        type="button"
        :disabled="!templateFilename || loading || repairing || migrating"
        :title="t('workspace.cleanup.migrate.description')"
        @click="migrate"
      >
        {{ migrating
          ? t('workspace.cleanup.migrating')
          : t('workspace.cleanup.migrate') }}
      </button>
      <button
        class="tool-btn"
        type="button"
        :disabled="loading || !templateFilename || repairing || migrating"
        @click="analyze"
      >
        {{ loading
          ? t('workspace.cleanup.analyzing')
          : t('workspace.cleanup.analyze') }}
      </button>
      <button
        class="tool-btn primary"
        type="button"
        :disabled="!repairBtnEnabled || migrating"
        @click="repair"
      >
        {{ repairing
          ? t('workspace.cleanup.repairing')
          : t('workspace.cleanup.repair') }}
      </button>
    </template>
  </Modal>
</template>
