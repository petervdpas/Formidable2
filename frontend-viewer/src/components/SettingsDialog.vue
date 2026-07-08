<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { api, type Config, type ServerStatus, type APIStatus } from "../api";
import { loadMessages } from "../i18n";
import { applyTheme } from "../theme";
import { reportError } from "../state";
import SelectField from "./SelectField.vue";
import SwitchField from "./SwitchField.vue";

const emit = defineEmits<{ close: [] }>();
const { t } = useI18n();

const cfg = ref<Config | null>(null);
const langs = ref<string[]>([]);
const status = ref<ServerStatus>({ running: false, port: 0, urls: [] });
const apiStat = ref<APIStatus>({ enabled: false, available: false, urls: [], token: "" });

const languageOptions = computed(() => [
  { value: "system", label: t("settings.language_system") },
  ...langs.value.map((l) => ({ value: l, label: l.toUpperCase() })),
]);
const themeOptions = computed(() => [
  { value: "system", label: t("settings.theme_system") },
  { value: "light", label: t("settings.theme_light") },
  { value: "dark", label: t("settings.theme_dark") },
]);

async function load(): Promise<void> {
  try {
    cfg.value = await api.getConfig();
    langs.value = await api.languages();
    status.value = await api.serverStatus();
    apiStat.value = await api.apiStatus();
  } catch (e) {
    reportError(e);
  }
}
onMounted(load);

// Persist + apply on every change so settings propagate instantly: no Save gate.
async function apply(): Promise<void> {
  if (!cfg.value) return;
  try {
    const applied = await api.setConfig(cfg.value);
    cfg.value = applied;
    applyTheme(applied.theme);
    status.value = await api.serverStatus();
    apiStat.value = await api.apiStatus();
    await loadMessages();
  } catch (e) {
    reportError(e);
  }
}

function setLanguage(v: string): void {
  if (cfg.value) {
    cfg.value.language = v;
    void apply();
  }
}
function setTheme(v: string): void {
  if (cfg.value) {
    cfg.value.theme = v;
    void apply();
  }
}
function setRememberSize(v: boolean): void {
  if (cfg.value) {
    cfg.value.remember_size = v;
    void apply();
  }
}
function setServeHTTP(v: boolean): void {
  if (cfg.value) {
    cfg.value.serve_http = v;
    void apply();
  }
}
function setServeAPI(v: boolean): void {
  if (cfg.value) {
    cfg.value.serve_api = v;
    void apply();
  }
}
async function generateToken(): Promise<void> {
  try {
    apiStat.value = await api.regenerateAPIToken();
    if (cfg.value) cfg.value.api_token = apiStat.value.token;
  } catch (e) {
    reportError(e);
  }
}
const copied = ref(false);
async function copyKey(): Promise<void> {
  try {
    await navigator.clipboard.writeText(cfg.value?.api_token ?? "");
    copied.value = true;
    setTimeout(() => (copied.value = false), 1500);
  } catch (e) {
    reportError(e);
  }
}

async function clearRecents(): Promise<void> {
  if (!cfg.value) return;
  try {
    cfg.value.recent_bundles = [];
    cfg.value = await api.setConfig(cfg.value);
  } catch (e) {
    reportError(e);
  }
}
</script>

<template>
  <div class="modal-backdrop" @click.self="emit('close')">
    <div class="modal" v-if="cfg">
      <h2 class="modal-title">{{ $t("settings.title") }}</h2>

      <div class="field">
        <label>{{ $t("settings.language") }}</label>
        <SelectField :model-value="cfg.language" :options="languageOptions" @update:model-value="setLanguage" />
      </div>

      <div class="field">
        <label>{{ $t("settings.theme") }}</label>
        <SelectField :model-value="cfg.theme" :options="themeOptions" @update:model-value="setTheme" />
      </div>

      <div class="switch-row">
        <SwitchField :model-value="cfg.remember_size" @update:model-value="setRememberSize" />
        <span>{{ $t("settings.remember_size") }}</span>
      </div>

      <div class="switch-row">
        <SwitchField :model-value="cfg.serve_http" @update:model-value="setServeHTTP" />
        <span>{{ $t("settings.serve_http") }}</span>
      </div>
      <p class="field-help">{{ $t("settings.serve_http_help") }}</p>

      <div class="field" v-if="cfg.serve_http">
        <label>{{ $t("settings.port") }}</label>
        <input type="number" min="1024" max="65535" v-model.number="cfg.http_port" @change="apply" />
      </div>

      <div class="lan" v-if="status.running && status.urls.length">
        <div class="lan-title">{{ $t("settings.lan_urls") }}</div>
        <ul>
          <li v-for="u in status.urls" :key="u"><code>{{ u }}</code></li>
        </ul>
      </div>

      <div class="switch-row">
        <SwitchField :model-value="cfg.serve_api" @update:model-value="setServeAPI" />
        <span>{{ $t("settings.serve_api") }}</span>
      </div>
      <p class="field-help">{{ $t("settings.serve_api_help") }}</p>

      <div class="field" v-if="cfg.serve_api">
        <label>{{ $t("settings.api_token") }}</label>
        <input type="text" spellcheck="false" v-model="cfg.api_token" @change="apply" />
        <div class="token-actions">
          <button class="btn ghost small" :disabled="!cfg.api_token" @click="copyKey">
            {{ copied ? $t("settings.api_copied") : $t("settings.api_copy") }}
          </button>
          <button class="btn ghost small" @click="generateToken">
            {{ $t("settings.api_generate") }}
          </button>
        </div>
        <p class="field-help">{{ $t("settings.api_token_help") }}</p>
      </div>

      <div class="lan" v-if="apiStat.enabled && apiStat.available && apiStat.urls.length">
        <div class="lan-title">{{ $t("settings.api_urls") }}</div>
        <ul>
          <li v-for="u in apiStat.urls" :key="u"><code>{{ u }}</code></li>
        </ul>
      </div>
      <p v-else-if="cfg.serve_api && !apiStat.available" class="field-help">
        {{ $t("settings.api_no_data") }}
      </p>

      <button class="btn ghost small" @click="clearRecents">{{ $t("settings.clear_recents") }}</button>

      <div class="modal-actions">
        <button class="btn primary" @click="emit('close')">{{ $t("settings.close") }}</button>
      </div>
    </div>
  </div>
</template>
