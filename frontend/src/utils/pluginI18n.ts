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
