import { i18n } from "../i18n";

// Helpers that resolve a plugin's user-facing strings through the
// `plugin.<id>.*` namespace populated by useI18nLoader, falling back
// to the literal manifest values, and finally to a stable id-based
// label so the UI never renders an empty string.
//
// Plugin authors ship a `<plugin-dir>/i18n/<locale>.json` whose
// (auto-prefixed) keys follow the convention:
//
//   "name"                          → plugin display name
//   "description"                   → plugin description
//   "commands.<cmd_id>.label"       → command label
//
// Components should prefer these helpers over reading
// `manifest.name` / `manifest.description` / `command.label`
// directly so translations land everywhere automatically.

type PluginLike = {
  id: string;
  manifest: { name?: string; description?: string };
};

type CommandLike = { id: string; label?: string };

function tIfExists(key: string): string | null {
  return i18n.global.te(key) ? (i18n.global.t(key) as string) : null;
}

export function pluginName(p: PluginLike): string {
  return tIfExists(`plugin.${p.id}.name`) ?? p.manifest.name ?? p.id;
}

export function pluginDescription(p: PluginLike): string {
  return tIfExists(`plugin.${p.id}.description`) ?? p.manifest.description ?? "";
}

export function commandLabel(pluginID: string, cmd: CommandLike): string {
  return (
    tIfExists(`plugin.${pluginID}.commands.${cmd.id}.label`) ??
    cmd.label ??
    cmd.id
  );
}

// Field-level i18n: a plugin form.json field declares a base key via
// `i18n: <key>` and the renderer resolves three sub-keys under the
// caller-supplied namespace (typically `plugin.<id>`). When the
// namespace is missing, when the field carries no `i18n`, or when
// the sub-key isn't translated in the active locale, the literal
// field value is returned.
//
// `namespace` is the auto-prefix the caller has already chosen for
// this rendering context; concrete value is `plugin.<id>` for plugin
// Run dialogs. Editor surfaces pass `null`/`""` to keep literal
// labels visible while authors are editing the manifest itself.

type FieldI18nSubKey = "label" | "description" | "placeholder";

function fieldI18nKey(namespace: string, baseKey: string, sub: FieldI18nSubKey): string {
  return `${namespace}.${baseKey}.${sub}`;
}

export function fieldLabel(
  namespace: string | null | undefined,
  field: { key: string; label?: string; i18n?: string },
): string {
  if (namespace && field.i18n) {
    const translated = tIfExists(fieldI18nKey(namespace, field.i18n, "label"));
    if (translated !== null) return translated;
  }
  return field.label || field.key;
}

export function fieldDescription(
  namespace: string | null | undefined,
  field: { description?: string; i18n?: string },
): string {
  if (namespace && field.i18n) {
    const translated = tIfExists(fieldI18nKey(namespace, field.i18n, "description"));
    if (translated !== null) return translated;
  }
  return field.description ?? "";
}

export function fieldPlaceholder(
  namespace: string | null | undefined,
  field: { i18n?: string },
  fallback = "",
): string {
  if (namespace && field.i18n) {
    const translated = tIfExists(fieldI18nKey(namespace, field.i18n, "placeholder"));
    if (translated !== null) return translated;
  }
  return fallback;
}
