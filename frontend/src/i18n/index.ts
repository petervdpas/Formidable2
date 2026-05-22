import { createI18n } from "vue-i18n";

// Empty messages at boot - populated by useI18nLoader once the Wails
// bridge is up. flatJson:true tells vue-i18n to treat dot-bearing keys
// as literal lookups (no nesting) so the Formidable-style flat key
// scheme (`status.ready`, `config.theme`, …) works directly.
export const i18n = createI18n({
  legacy: false,
  flatJson: true,
  fallbackLocale: "en",
  locale: "en",
  messages: {},
  // Keys that resolve to themselves on miss are loud enough; turn off
  // the warning so production logs stay clean.
  missingWarn: false,
  fallbackWarn: false,
});

export type LocaleId = string;
