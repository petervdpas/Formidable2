import { createI18n } from "vue-i18n";
import { api } from "./api";

export const i18n = createI18n({
  legacy: false,
  locale: "en",
  fallbackLocale: "en",
  messages: { en: {} },
});

// unflatten turns the backend's dotted keys ("settings.title") into the nested
// object vue-i18n resolves as a path.
function unflatten(flat: Record<string, string>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(flat)) {
    const parts = k.split(".");
    let cur = out;
    for (let i = 0; i < parts.length - 1; i++) {
      cur[parts[i]] = (cur[parts[i]] as Record<string, unknown>) || {};
      cur = cur[parts[i]] as Record<string, unknown>;
    }
    cur[parts[parts.length - 1]] = v;
  }
  return out;
}

// loadMessages fetches strings for a language (or the effective one) from the
// backend and installs them. The backend owns all copy; nothing is hardcoded.
export async function loadMessages(lang?: string): Promise<void> {
  const l = lang || (await api.effectiveLanguage());
  const flat = await api.messages(l);
  i18n.global.setLocaleMessage(l, unflatten(flat));
  (i18n.global.locale as { value: string }).value = l;
}
