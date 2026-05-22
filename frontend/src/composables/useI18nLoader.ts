import { ref, watch } from "vue";
import { Service as I18nSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/i18n";
import { Service as PluginSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";
import { i18n } from "../i18n";
import { useConfig } from "./useConfig";

const loaded = new Set<string>();
const availableLocales = ref<string[]>([]);
const defaultLocale = ref<string>("en");
let bootPromise: Promise<void> | null = null;

async function fetchPluginMessages(locale: string): Promise<void> {
  try {
    const msgs = await PluginSvc.GetI18nMessages(locale);
    if (msgs && Object.keys(msgs).length > 0) {
      i18n.global.mergeLocaleMessage(locale, msgs as Record<string, unknown>);
    }
  } catch (err) {
    // Plugin-side i18n is best-effort - a broken plugin must not
    // poison the core locale path.
    console.warn("plugin i18n fetch failed", { locale, err });
  }
}

async function ensureLocale(locale: string): Promise<void> {
  if (!loaded.has(locale)) {
    const bundle = await I18nSvc.LoadBundle(locale);
    i18n.global.setLocaleMessage(locale, bundle as Record<string, unknown>);
    loaded.add(locale);
  }
  await fetchPluginMessages(locale);
}

// Called by usePlugins after refresh / install / delete so newly-
// added plugin messages reach vue-i18n without an app restart.
export async function refreshPluginI18n(): Promise<void> {
  const active = i18n.global.locale.value as string;
  await fetchPluginMessages(active);
  const fallback = defaultLocale.value;
  if (fallback && fallback !== active && loaded.has(fallback)) {
    await fetchPluginMessages(fallback);
  }
}

async function boot(): Promise<void> {
  if (bootPromise) return bootPromise;
  bootPromise = (async () => {
    const [locales, def] = await Promise.all([
      I18nSvc.AvailableLocales(),
      I18nSvc.DefaultLocale(),
    ]);
    availableLocales.value = locales;
    defaultLocale.value = def;

    // Always preload the default; that's the fallback for any missing key.
    await ensureLocale(def);

    // Then preload whatever the active config says - config may not be
    // ready yet, in which case the watcher below picks it up.
    const { config } = useConfig();
    const wantLocale = config.value?.language;
    if (wantLocale && wantLocale !== def && locales.includes(wantLocale)) {
      await ensureLocale(wantLocale);
    }
    setActive(wantLocale ?? def);
  })();
  return bootPromise;
}

function setActive(locale: string) {
  if (loaded.has(locale)) {
    i18n.global.locale.value = locale;
  } else {
    // Lazy-load on demand, then activate.
    ensureLocale(locale)
      .then(() => { i18n.global.locale.value = locale; })
      .catch(() => { i18n.global.locale.value = defaultLocale.value; });
  }
}

// Wire config.language → active locale. Single subscription at module
// scope so additional component callers don't multiply the watcher.
const { config } = useConfig();
watch(
  () => config.value?.language,
  (lang) => {
    if (!lang) return;
    if (!availableLocales.value.includes(lang)) return;
    setActive(lang);
  },
  { immediate: false },
);

export function useI18nLoader() {
  // Calling this kicks off the boot once. Components that need to wait
  // for the first bundle can `await ensureI18nReady()`.
  if (!bootPromise) boot();
  return {
    availableLocales,
    defaultLocale,
    ensureI18nReady: () => boot(),
  };
}
