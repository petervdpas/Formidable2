<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Service as PluginSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";
import { Service as I18nSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/i18n";
import { useToast } from "../composables/useToast";
import { refreshPluginI18n } from "../composables/useI18nLoader";
import { backendErrMessage } from "../utils/backendError";

// PluginI18nEditor - per-locale key/value editor for one plugin's
// <plugin>/i18n/<locale>.json files. Locale switcher up top, table
// of {key, value} rows, plus a "+ Add locale" row that surfaces
// every known backend locale the plugin doesn't yet have a file
// for. Saving any locale re-fetches the runtime-merged plugin i18n
// so the rest of the UI updates without a workspace remount.

const props = defineProps<{ pluginId: string }>();

const { t } = useI18n();
const toast = useToast();

type Row = { key: string; value: string };

const locales = ref<string[]>([]);
const activeLocale = ref<string>("");
const rows = ref<Row[]>([]);
const baselineRows = ref<Row[]>([]);

const allBackendLocales = ref<string[]>([]);
const addLocaleInput = ref<string>("");
const showAddLocale = ref<boolean>(false);

const dirty = computed<boolean>(() => {
  if (rows.value.length !== baselineRows.value.length) return true;
  for (let i = 0; i < rows.value.length; i++) {
    if (
      rows.value[i].key !== baselineRows.value[i].key ||
      rows.value[i].value !== baselineRows.value[i].value
    ) {
      return true;
    }
  }
  return false;
});

const knownLocaleSuggestions = computed<string[]>(() =>
  allBackendLocales.value.filter((l) => !locales.value.includes(l)),
);

async function loadLocales(): Promise<void> {
  if (!props.pluginId) {
    locales.value = [];
    activeLocale.value = "";
    rows.value = [];
    baselineRows.value = [];
    return;
  }
  try {
    locales.value = (await PluginSvc.ListPluginLocales(props.pluginId)) ?? [];
  } catch (err) {
    toast.error(backendErrMessage(err));
    locales.value = [];
  }
  if (locales.value.length === 0) {
    activeLocale.value = "";
    rows.value = [];
    baselineRows.value = [];
    return;
  }
  if (!activeLocale.value || !locales.value.includes(activeLocale.value)) {
    activeLocale.value = locales.value[0];
  }
  await loadActive();
}

async function loadActive(): Promise<void> {
  if (!props.pluginId || !activeLocale.value) return;
  try {
    const msgs = await PluginSvc.GetPluginI18n(props.pluginId, activeLocale.value);
    const next: Row[] = Object.entries(msgs ?? {})
      .map(([key, value]) => ({ key, value: String(value) }))
      .sort((a, b) => a.key.localeCompare(b.key));
    rows.value = next;
    baselineRows.value = clone(next);
  } catch (err) {
    toast.error(backendErrMessage(err));
    rows.value = [];
    baselineRows.value = [];
  }
}

async function loadBackendLocales(): Promise<void> {
  try {
    allBackendLocales.value = (await I18nSvc.AvailableLocales()) ?? [];
  } catch {
    allBackendLocales.value = [];
  }
}

function clone(arr: Row[]): Row[] {
  return arr.map((r) => ({ key: r.key, value: r.value }));
}

function addRow(): void {
  rows.value.push({ key: "", value: "" });
}

function removeRow(idx: number): void {
  rows.value.splice(idx, 1);
}

async function selectLocale(locale: string): Promise<void> {
  if (locale === activeLocale.value) return;
  if (dirty.value) {
    // Quietly drop the in-progress edits; the user can always click
    // Save before switching. A confirm prompt would feel heavy for
    // a tab they can flip back to in a second.
  }
  activeLocale.value = locale;
  await loadActive();
}

async function addLocale(localeID: string): Promise<void> {
  const id = localeID.trim();
  if (!id) return;
  if (!/^[A-Za-z0-9_-]{1,32}$/.test(id)) {
    toast.error(`Invalid locale id "${id}". Use letters, digits, dash, underscore.`);
    return;
  }
  if (locales.value.includes(id)) {
    activeLocale.value = id;
    await loadActive();
    showAddLocale.value = false;
    addLocaleInput.value = "";
    return;
  }
  // Persist an empty file so ListPluginLocales picks it up next refresh.
  try {
    await PluginSvc.SavePluginI18n(props.pluginId, id, {});
    locales.value = [...locales.value, id].sort();
    activeLocale.value = id;
    rows.value = [];
    baselineRows.value = [];
    showAddLocale.value = false;
    addLocaleInput.value = "";
    await refreshPluginI18n();
  } catch (err) {
    toast.error(backendErrMessage(err));
  }
}

async function save(): Promise<void> {
  if (!props.pluginId || !activeLocale.value) return;
  // Strip empty-key rows so a half-typed row doesn't poison the file.
  // Duplicate keys collapse to the last-wins value, matching JSON
  // object semantics on read.
  const msgs: Record<string, string> = {};
  for (const r of rows.value) {
    const k = r.key.trim();
    if (!k) continue;
    msgs[k] = r.value;
  }
  try {
    await PluginSvc.SavePluginI18n(props.pluginId, activeLocale.value, msgs);
    baselineRows.value = clone(rows.value);
    await refreshPluginI18n();
    toast.success(t("workspace.plugins.i18n.save_success", [activeLocale.value]));
  } catch (err) {
    toast.error(backendErrMessage(err));
  }
}

async function deleteLocale(): Promise<void> {
  if (!props.pluginId || !activeLocale.value) return;
  const locale = activeLocale.value;
  const ok = window.confirm(
    t("workspace.plugins.i18n.delete_locale_confirm", [locale]),
  );
  if (!ok) return;
  try {
    await PluginSvc.DeletePluginI18n(props.pluginId, locale);
    locales.value = locales.value.filter((l) => l !== locale);
    activeLocale.value = locales.value[0] ?? "";
    rows.value = [];
    baselineRows.value = [];
    if (activeLocale.value) await loadActive();
    await refreshPluginI18n();
  } catch (err) {
    toast.error(backendErrMessage(err));
  }
}

watch(
  () => props.pluginId,
  () => {
    activeLocale.value = "";
    void loadLocales();
  },
  { immediate: true },
);

void loadBackendLocales();
</script>

<template>
  <div class="plugin-i18n-editor">
    <p class="muted small i18n-help">
      {{ t("workspace.plugins.i18n.help", [props.pluginId || "<id>"]) }}
    </p>

    <div class="locale-switcher">
      <button
        v-for="loc in locales"
        :key="loc"
        type="button"
        :class="['locale-chip', { active: loc === activeLocale }]"
        @click="selectLocale(loc)"
      >
        {{ loc }}
      </button>
      <button
        v-if="!showAddLocale"
        type="button"
        class="locale-chip locale-chip-add"
        @click="showAddLocale = true"
      >
        {{ t("workspace.plugins.i18n.add_locale") }}
      </button>
      <template v-else>
        <select
          v-if="knownLocaleSuggestions.length > 0"
          v-model="addLocaleInput"
          class="locale-add-select"
        >
          <option value="">{{ t("workspace.plugins.i18n.add_locale_placeholder") }}</option>
          <option v-for="loc in knownLocaleSuggestions" :key="loc" :value="loc">{{ loc }}</option>
        </select>
        <input
          v-else
          v-model="addLocaleInput"
          class="locale-add-input"
          :placeholder="t('workspace.plugins.i18n.add_locale_placeholder')"
          @keydown.enter.prevent="addLocale(addLocaleInput)"
        />
        <button
          type="button"
          class="tool-btn small"
          :disabled="!addLocaleInput.trim()"
          @click="addLocale(addLocaleInput)"
        >
          +
        </button>
        <button
          type="button"
          class="tool-btn small"
          @click="showAddLocale = false; addLocaleInput = ''"
        >
          ×
        </button>
      </template>
    </div>

    <p v-if="locales.length === 0" class="muted small">
      {{ t("workspace.plugins.i18n.empty") }}
    </p>

    <template v-else-if="activeLocale">
      <div class="i18n-prefix">
        <span class="meta-key small">{{ t("workspace.plugins.i18n.prefix_label") }}</span>
        <code class="meta-value mono">plugin.{{ pluginId }}.</code>
      </div>

      <table v-if="rows.length > 0" class="i18n-table">
        <thead>
          <tr>
            <th>{{ t("workspace.plugins.i18n.key") }}</th>
            <th>{{ t("workspace.plugins.i18n.value") }}</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(row, idx) in rows" :key="idx">
            <td>
              <input
                v-model="row.key"
                class="field-input mono"
                :placeholder="t('workspace.plugins.i18n.key_placeholder')"
              />
            </td>
            <td>
              <input
                v-model="row.value"
                class="field-input"
                :placeholder="t('workspace.plugins.i18n.value_placeholder')"
              />
            </td>
            <td>
              <button
                type="button"
                class="btn-ghost-icon"
                @click="removeRow(idx)"
                title="Remove"
              >−</button>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-else class="muted small">{{ t("workspace.plugins.i18n.no_rows") }}</p>

      <div class="i18n-actions">
        <button type="button" class="tool-btn" @click="addRow">
          {{ t("workspace.plugins.i18n.add_row") }}
        </button>
        <button
          type="button"
          class="tool-btn primary"
          :disabled="!dirty"
          @click="save"
        >
          {{ t("workspace.plugins.i18n.save") }}
        </button>
        <button
          type="button"
          class="tool-btn danger"
          @click="deleteLocale"
        >
          {{ t("workspace.plugins.i18n.delete_locale") }}
        </button>
      </div>
    </template>
  </div>
</template>
