<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import FormFieldRow from "./form-fields/FormFieldRow.vue";
import PluginResultPanel from "./PluginResultPanel.vue";
import {
  Service as PluginSvc,
  Command,
  RunResultDTO,
  type ListResult,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";
import { Service as RenderSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/render";
import type { Field } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { getFieldTypeDef } from "../types/field-types";
import {
  useGlobalPluginRun,
  closeGlobalPluginRun,
  setGlobalPluginRunning,
  cancelGlobalPluginRun,
} from "../composables/useGlobalPluginRun";
import { useToast } from "../composables/useToast";

// PluginRunDialog mirrors PluginsWorkspace's inline Run modal but is
// mounted once at the App level and driven entirely by the manifest
// of the plugin in `openRequest`. The two modes the manifest declares
// are honored verbatim — there is no filtering on top:
//   - run_mode === "form" → render the plugin's form (with the
//     description above it) and each form_button command as an action
//     button. ctx = { ...extraCtx, ...formValues }.
//   - else                → render one card per non-form_button
//     command, ctx = { ...extraCtx }.
//
// Workspace topbar menu items pass extraCtx = { workspace: "<id>" } so
// the Lua side can branch on its invocation context.

const { t } = useI18n();
const toast = useToast();
const { openRequest, running, stopping, progress } = useGlobalPluginRun();

const plugin = computed<ListResult | null>(() => openRequest.value?.plugin ?? null);
const extraCtx = computed<Record<string, unknown>>(() => openRequest.value?.extraCtx ?? {});

const formFields = ref<Field[]>([]);
const runValues = ref<Record<string, unknown>>({});
const runResults = ref<Record<string, RunResultDTO>>({});
const runningCmd = ref<string>("");
const descriptionHTML = ref<string>("");

const runMode = computed(
  () => (plugin.value?.manifest.run_mode || "modal") as "modal" | "form",
);

const visibleCommands = computed(() => {
  const all = plugin.value?.manifest.commands ?? [];
  if (runMode.value === "form") {
    return all.filter((c) => c.form_button);
  }
  return all.filter((c) => !c.form_button);
});

function initialRunValues(fields: Field[]): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const f of fields) {
    if (!f.key) continue;
    if (f.default !== undefined && f.default !== null) {
      out[f.key] = f.default;
      continue;
    }
    const def = getFieldTypeDef(f.type)?.defaultValue?.();
    out[f.key] = def !== undefined ? def : "";
  }
  return out;
}

// Open → reset state, load form schema + KV-saved values from disk.
// Cleared on close (openRequest goes null) so the next open doesn't
// flash stale data.
watch(
  plugin,
  async (p) => {
    runResults.value = {};
    runningCmd.value = "";
    if (!p) {
      formFields.value = [];
      runValues.value = {};
      descriptionHTML.value = "";
      return;
    }
    let fields: Field[] = [];
    try {
      const raw = await PluginSvc.GetForm(p.id);
      const parsed = JSON.parse(raw || "[]");
      if (Array.isArray(parsed)) {
        fields = parsed;
      } else if (
        parsed &&
        typeof parsed === "object" &&
        Array.isArray((parsed as { fields?: unknown }).fields)
      ) {
        fields = (parsed as { fields: Field[] }).fields;
      }
    } catch {
      fields = [];
    }
    formFields.value = fields;
    let values = initialRunValues(fields);
    const keys = fields.map((f) => f.key).filter((k): k is string => !!k);
    if (keys.length > 0) {
      try {
        const saved = await PluginSvc.LoadFormValues(p.id, keys);
        for (const k of keys) {
          if (saved && saved[k] !== undefined) values[k] = saved[k];
        }
      } catch {
        /* fall back to defaults */
      }
    }
    runValues.value = values;

    const md = p.manifest.description?.trim() ?? "";
    if (md) {
      try {
        descriptionHTML.value = await RenderSvc.RenderHTML(md);
      } catch {
        descriptionHTML.value = md;
      }
    } else {
      descriptionHTML.value = "";
    }
  },
  { immediate: true },
);

async function runCommand(cmd: Command) {
  const p = plugin.value;
  if (!p) return;
  runningCmd.value = cmd.id;
  setGlobalPluginRunning(true);
  try {
    const isForm = runMode.value === "form";
    const ctx: Record<string, unknown> = isForm
      ? { ...extraCtx.value, ...runValues.value }
      : { ...extraCtx.value };
    if (isForm) {
      try {
        await PluginSvc.SaveFormValues(p.id, { ...runValues.value });
      } catch {
        /* best-effort; don't block the run */
      }
    }
    const res = await PluginSvc.Run(p.id, cmd.id, ctx);
    runResults.value[cmd.id] = res;
    if (res.kind === "busy") {
      toast.warn(res.message || "plugin: another command is currently running");
    } else if (res.kind === "cancelled") {
      toast.info("workspace.plugins.cancelled");
    }
    for (const ev of res.toasts ?? []) {
      const fn = toast[ev.level as "info" | "success" | "warn" | "error"];
      if (fn) fn(ev.message);
    }
    if (cmd.log_as_toast) {
      for (const line of res.logLines ?? []) {
        const m = /^\[(\w+)\]\s*(.*)$/.exec(line);
        const level = (m?.[1] ?? "info").toLowerCase();
        const msg = m?.[2] ?? line;
        const variant: "info" | "success" | "warn" | "error" =
          level === "warn"
            ? "warn"
            : level === "error"
              ? "error"
              : "info";
        toast[variant](msg);
      }
    }
  } catch (err) {
    runResults.value[cmd.id] = new RunResultDTO({
      kind: "runtime_error",
      message: String(err),
    });
  } finally {
    runningCmd.value = "";
    setGlobalPluginRunning(false);
  }
}

function close() {
  closeGlobalPluginRun();
}

async function stop() {
  await cancelGlobalPluginRun();
}

const progressPct = computed<number>(() => {
  const p = progress.value;
  if (!p || p.total <= 0) return 0;
  return Math.max(0, Math.min(100, Math.round((p.done / p.total) * 100)));
});

const progressIsIndeterminate = computed<boolean>(() => {
  const p = progress.value;
  return !!p && p.total <= 0;
});

const showProgressBar = computed<boolean>(
  () => plugin.value?.manifest.progress === true,
);
</script>

<template>
  <Modal
    :open="!!plugin"
    :title="
      plugin
        ? runMode === 'form'
          ? plugin.manifest.name || plugin.id
          : t('workspace.plugins.run_title', [plugin.manifest.name || plugin.id])
        : ''
    "
    width="640px"
    @close="close"
  >
    <div v-if="plugin" class="run-modal">
      <section v-if="runMode === 'form'" class="run-form">
        <div
          v-if="descriptionHTML"
          class="section-info"
          v-html="descriptionHTML"
        ></div>
        <FormFieldRow
          v-for="(f, i) in formFields"
          :key="f.key || i"
          :field="f"
          :model-value="runValues[f.key]"
          @update:model-value="(v: unknown) => (runValues[f.key] = v)"
        />
        <div
          v-if="visibleCommands.length > 0"
          class="run-form-buttons"
        >
          <button
            v-for="cmd in visibleCommands"
            :key="cmd.id"
            class="tool-btn primary"
            :disabled="running"
            @click="runCommand(cmd)"
          >
            <span v-if="runningCmd === cmd.id">
              {{ t('workspace.plugins.running') }}
            </span>
            <span v-else>{{ cmd.label || cmd.id }}</span>
          </button>
        </div>
      </section>

      <template v-else>
        <section
          v-for="cmd in visibleCommands"
          :key="cmd.id"
          class="command-card"
        >
          <div class="command-header">
            <h3>{{ cmd.label || cmd.id }}</h3>
            <button
              class="tool-btn primary"
              :disabled="running"
              @click="runCommand(cmd)"
            >
              <span v-if="runningCmd === cmd.id">
                {{ t('workspace.plugins.running') }}
              </span>
              <span v-else>{{ t('workspace.plugins.run') }}</span>
            </button>
          </div>
        </section>
      </template>

      <div v-if="running" class="plugin-run-progress">
        <div v-if="showProgressBar && progress?.stage" class="plugin-run-progress-stage">
          {{ progress.stage }}
        </div>
        <div class="plugin-run-progress-row">
          <div
            v-if="showProgressBar"
            class="plugin-run-progress-bar"
            :class="{ 'is-indeterminate': progressIsIndeterminate }"
          >
            <div
              class="plugin-run-progress-fill"
              :style="!progressIsIndeterminate ? { width: progressPct + '%' } : undefined"
            ></div>
          </div>
          <span v-else class="muted small plugin-run-progress-running">
            {{ t('workspace.plugins.running') }}
          </span>
          <button
            class="tool-btn"
            type="button"
            :disabled="stopping"
            @click="stop"
          >
            {{ stopping ? t('workspace.plugins.stopping') : t('workspace.plugins.stop') }}
          </button>
        </div>
        <p v-if="showProgressBar" class="plugin-run-progress-label">
          <span v-if="progress && progress.total > 0" class="plugin-run-progress-count">
            {{ progress.done }} / {{ progress.total }}
          </span>
          <span v-if="progress?.message" class="plugin-run-progress-msg">
            {{ progress.message }}
          </span>
        </p>
      </div>

      <PluginResultPanel
        :commands="visibleCommands"
        :results="runResults"
        :enabled="!!plugin.manifest.debug"
      />
    </div>

    <template #footer>
      <button class="tool-btn" type="button" @click="close">
        {{ t('common.close') }}
      </button>
    </template>
  </Modal>
</template>
