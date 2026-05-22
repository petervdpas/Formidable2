<script setup lang="ts">
import {
  computed,
  inject,
  ref,
  watch,
  type ComputedRef,
} from "vue";
import { useI18n } from "vue-i18n";
import { TextField, SelectField } from "../fields";
import ConfirmDialog from "../ConfirmDialog.vue";
import { Service as ConfigSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/config";
import { Service as FormSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/form";
import { useFormidableLink } from "../../composables/useFormidableLink";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// FormFieldLink - composes a `{href, text}` from either a free-form
// URL (regular protocol) or a template + entry pair (formidable://).
//
// Storage shape:
//   ""                                - empty value
//   { href, text }                    - canonical
//   "https://…" | "formidable://…"    - legacy string accepted on read
//
// Two-way binding strategy:
//   - Local refs hold protocol/url/tplPick/entryPick/text so the user
//     can stay in an "intermediate" state (e.g. switched protocol but
//     hasn't picked an entry yet) without losing partially-typed input.
//   - syncFromModelValue() re-derives those refs from props.modelValue.
//     Called once at setup, and again every time the parent passes a
//     different value - that catches form-switch, where Vue reuses the
//     component instance (same template position, different datafile)
//     and only the prop changes.
//   - The echo-skip in the modelValue watcher avoids wiping refs when
//     OUR own emit propagates back through the prop in the same tick.

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const { t } = useI18n();

// Provided by StorageWorkspace - used to default the template picker
// to the form's own template (mirrors original Formidable behavior).
const templateFilename = inject<ComputedRef<string>>(
  "templateFilename",
  computed(() => ""),
);

// ── value shape helpers ──────────────────────────────────────────────
type LinkValue = { href: string; text: string };
type Protocol = "regular" | "formidable";

function normalize(v: unknown): LinkValue {
  if (!v) return { href: "", text: "" };
  if (typeof v === "string") return { href: v, text: "" };
  if (typeof v === "object") {
    const o = v as Record<string, unknown>;
    return {
      href: typeof o.href === "string" ? o.href : "",
      text: typeof o.text === "string" ? o.text : "",
    };
  }
  return { href: "", text: "" };
}

function parseFormidableHref(href: string): { template: string; entry: string } | null {
  if (!href.startsWith("formidable://")) return null;
  const rest = href.slice("formidable://".length);
  const idx = rest.lastIndexOf(":");
  if (idx <= 0) return null;
  return { template: rest.slice(0, idx), entry: rest.slice(idx + 1) };
}

// ── reactive state ───────────────────────────────────────────────────
const protocol = ref<Protocol>("regular");
const url = ref<string>("");
const tplPick = ref<string>("");
const entryPick = ref<string>("");
const text = ref<string>("");
// True once the user has typed in the text field. Until then we
// auto-fill text from the URL or chosen entry.
const userTouchedText = ref<boolean>(false);

function syncFromModelValue(v: unknown) {
  const n = normalize(v);
  const f = parseFormidableHref(n.href);
  protocol.value = f ? "formidable" : "regular";
  url.value = f ? "" : n.href;
  tplPick.value = f?.template ?? "";
  entryPick.value = f?.entry ?? "";
  text.value = n.text;
  userTouchedText.value = n.text !== "";
}

// Initialize from current prop value (form load).
syncFromModelValue(props.modelValue);

// ── option lists ─────────────────────────────────────────────────────
const protocolOptions = computed(() => [
  { value: "regular", label: "regular" },
  { value: "formidable", label: "formidable" },
]);

const templates = ref<string[]>([]);
const entries = ref<string[]>([]);
const loadingEntries = ref(false);

const templateOptions = computed(() =>
  templates.value.map((name) => ({ value: name, label: name })),
);
const entryOptions = computed(() =>
  entries.value.map((name) => ({ value: name, label: name })),
);

// ── href composition ─────────────────────────────────────────────────
const composedHref = computed<string>(() => {
  if (protocol.value === "regular") return url.value.trim();
  if (!tplPick.value || !entryPick.value) return "";
  return `formidable://${tplPick.value}:${entryPick.value}`;
});

const barePreview = computed<string>(() => composedHref.value);

// What the field would emit right now, given current refs. Used to
// detect echoes of our own emit so the modelValue watcher doesn't
// wipe in-flight ref state.
function currentEmitShape(): unknown {
  const href = composedHref.value;
  const txt = text.value.trim();
  if (!href && !txt) return "";
  return { href, text: txt };
}

function structurallyEqual(a: unknown, b: unknown): boolean {
  return JSON.stringify(a ?? "") === JSON.stringify(b ?? "");
}

// ── data fetching ────────────────────────────────────────────────────
async function loadTemplates() {
  try {
    // Use the enabled subset, not the raw template list - the link
    // picker is a use-side surface and should respect per-profile
    // curation. The backend method already self-heals against the
    // live folder, so no separate prune pass is needed here.
    const list = await ConfigSvc.ListEnabledTemplates();
    templates.value = (list ?? []).filter((s): s is string => typeof s === "string");
    if (!tplPick.value) {
      // Prefer the form's own template when it's still enabled; fall
      // back to the first allowed template otherwise.
      const own = templateFilename.value;
      if (own && templates.value.includes(own)) {
        tplPick.value = own;
      } else {
        tplPick.value = templates.value[0] ?? "";
      }
    } else if (!templates.value.includes(tplPick.value)) {
      // A previously-picked template was disabled - fall back to the
      // first allowed one (or empty when nothing's enabled).
      tplPick.value = templates.value[0] ?? "";
    }
  } catch {
    templates.value = [];
  }
}

async function loadEntries(forTemplate: string) {
  if (!forTemplate) {
    entries.value = [];
    return;
  }
  loadingEntries.value = true;
  try {
    const list = await FormSvc.ListForms(forTemplate);
    entries.value = (list ?? []).map((s) => s.filename);
  } catch {
    entries.value = [];
  } finally {
    loadingEntries.value = false;
  }
}

// Load templates + entries the first time formidable mode becomes
// active (and on each subsequent re-entry - reload is cheap).
watch(
  protocol,
  async (p) => {
    if (p !== "formidable") return;
    if (templates.value.length === 0) await loadTemplates();
    if (tplPick.value && entries.value.length === 0) {
      await loadEntries(tplPick.value);
    }
  },
  { immediate: true },
);

// Re-load entries when template changes.
watch(tplPick, async (next, prev) => {
  if (protocol.value !== "formidable") return;
  if (next === prev) return;
  await loadEntries(next);
  if (entryPick.value && !entries.value.includes(entryPick.value)) {
    entryPick.value = entries.value[0] ?? "";
  }
  if (!userTouchedText.value && entryPick.value) {
    text.value = entryPick.value.replace(/\.meta\.json$/i, "");
  }
});

watch(entryPick, (next) => {
  if (!userTouchedText.value && next) {
    text.value = next.replace(/\.meta\.json$/i, "");
  }
});

// Auto-fill text from URL while user hasn't typed it.
watch(url, (next) => {
  if (!userTouchedText.value) text.value = next.trim();
});

function onTextInput() {
  userTouchedText.value = true;
}

const canClear = computed(
  () => !props.field.readonly && (composedHref.value !== "" || text.value !== ""),
);

// Reset every part of the link except the protocol - the user's chosen
// mode is a UI affordance, not part of the saved value, and clobbering
// it forces them back through the protocol picker just to start over.
const confirmClearOpen = ref(false);
function requestClear() {
  if (!canClear.value) return;
  confirmClearOpen.value = true;
}
function cancelClear() {
  confirmClearOpen.value = false;
}
function confirmClear() {
  url.value = "";
  tplPick.value = "";
  entryPick.value = "";
  text.value = "";
  userTouchedText.value = false;
  confirmClearOpen.value = false;
}

// ── follow bare-link click ───────────────────────────────────────────
const { follow: followFormidable } = useFormidableLink();

async function onBareClick(e: MouseEvent) {
  const href = composedHref.value;
  if (!href) return;
  if (href.startsWith("formidable://")) {
    e.preventDefault();
    await followFormidable(href);
  }
}

// ── re-sync when the PARENT passes a different value ────────────────
// Skip echoes of our own emit (incoming value structurally matches
// what we just emitted). Without this guard, a protocol switch would
// nuke tplPick/entryPick the moment the parent re-passes our emitted
// {href:"", text:"X"} - because parsing "" yields no formidable hint.
watch(
  () => props.modelValue,
  (v) => {
    if (structurallyEqual(v, currentEmitShape())) return;
    syncFromModelValue(v);
  },
);

// ── emit on user changes ─────────────────────────────────────────────
watch([composedHref, text], () => {
  emit("update:modelValue", currentEmitShape());
});
</script>

<template>
  <div class="link-field" :data-link-field="field.key">
    <!-- Row 1: protocol + (url | template + entry) -->
    <div class="link-field-row">
      <div class="link-field-stack">
        <label class="stacked-label">
          {{ t("field.link.protocol") }}
          <small class="label-subtext">{{ t("field.link.protocol.hint") }}</small>
        </label>
        <SelectField
          v-model="protocol"
          :options="protocolOptions"
          :disabled="field.readonly"
        />
      </div>

      <div v-if="protocol === 'regular'" class="link-field-stack link-field-stack-grow">
        <label class="stacked-label">
          {{ t("field.link.url") }}
          <small class="label-subtext">{{ t("field.link.url.hint") }}</small>
        </label>
        <TextField
          v-model="url"
          type="url"
          placeholder="https://… or formidable://…"
          :readonly="field.readonly"
        />
      </div>

      <template v-else>
        <div class="link-field-stack">
          <label class="stacked-label">
            {{ t("field.link.template") }}
            <small class="label-subtext">{{ t("field.link.template.hint") }}</small>
          </label>
          <SelectField
            v-model="tplPick"
            :options="templateOptions"
            :disabled="field.readonly"
          />
        </div>
        <div class="link-field-stack">
          <label class="stacked-label">
            {{ t("field.link.entry") }}
            <small class="label-subtext">{{ t("field.link.entry.hint") }}</small>
          </label>
          <SelectField
            v-model="entryPick"
            :options="entryOptions"
            :disabled="field.readonly || loadingEntries"
          />
        </div>
      </template>
    </div>

    <!-- Row 2: link text -->
    <div class="link-field-text">
      <label class="stacked-label">
        {{ t("field.link.text") }}
        <small class="label-subtext">{{ t("field.link.text.hint") }}</small>
      </label>
      <TextField
        v-model="text"
        :readonly="field.readonly"
        @input="onTextInput"
      />
    </div>

    <!-- Row 3: bare-link preview -->
    <div class="link-field-bare">
      <div class="link-field-bare-header">
        <label class="stacked-label">{{ t("field.link.bare") }}</label>
        <button
          type="button"
          class="tool-btn danger link-field-clear"
          :disabled="!canClear"
          @click="requestClear"
        >
          {{ t("common.clear") }}
        </button>
      </div>
      <div v-if="barePreview" class="link-field-bare-preview">
        <a
          :href="barePreview"
          target="_blank"
          rel="noopener noreferrer"
          @click="onBareClick"
        >
          {{ barePreview }}
        </a>
      </div>
      <div v-else class="link-field-bare-empty">
        {{ t("field.link.none") }}
      </div>
    </div>

    <ConfirmDialog
      :open="confirmClearOpen"
      :title="t('field.link.clear.title')"
      :message="t('field.link.clear.confirm')"
      :confirm-label="t('common.clear')"
      :cancel-label="t('common.cancel')"
      variant="danger"
      @cancel="cancelClear"
      @confirm="confirmClear"
    />
  </div>
</template>
