import {
  Service as PluginSvc,
  type RunResultDTO,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";
import { askVaultUnlock } from "./useVaultUnlockPrompt";

// runPluginCommand is PluginSvc.Run with one courtesy: when the backend
// refuses because an api-client of the plugin keeps its credential in a locked
// vault, the user is asked for the master password right there and the
// command is run again. Cancelling the prompt hands back the refusal, so the
// caller shows why nothing ran.
export async function runPluginCommand(
  pluginID: string,
  commandID: string,
  ctx: Record<string, unknown>,
): Promise<RunResultDTO> {
  const res = await PluginSvc.Run(pluginID, commandID, ctx);
  if (res.kind !== "vault_locked") return res;
  if (!(await askVaultUnlock())) return res;
  return PluginSvc.Run(pluginID, commandID, ctx);
}
