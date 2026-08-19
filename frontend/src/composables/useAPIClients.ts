import { computed, ref } from "vue";
import {
  Service as ClientSvc,
  type Catalog,
  type ClientDetail,
  type Connection,
  type FetchRequest,
  type Item,
  type ListRequest,
  type Page,
  type Summary,
  type ValidationError,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/connection";
import { backendErrMessage } from "../utils/backendError";

// Module-scope so the panel keeps its selection when the user clicks to a
// sibling Information section and back, matching useFonts and useVault.
const summaries = ref<Summary[]>([]);
const detail = ref<ClientDetail | null>(null);
const loading = ref(false);
const lastError = ref("");

// Enumerations come from the backend rather than being restated here, so a new
// dialect or paging style appears in the pickers without a frontend change.
const dialects = ref<string[]>([]);
const keyStyles = ref<string[]>([]);
const paginationStyles = ref<string[]>([]);

type Result = { ok: boolean; message: string };

const ok = (): Result => ({ ok: true, message: "" });
const failed = (err: unknown): Result => ({ ok: false, message: backendErrMessage(err) });

async function loadEnums(): Promise<void> {
  if (dialects.value.length > 0) return;
  dialects.value = (await ClientSvc.ListDialects()) ?? [];
  keyStyles.value = (await ClientSvc.ListKeyStyles()) ?? [];
  paginationStyles.value = (await ClientSvc.ListPaginationStyles()) ?? [];
}

async function refresh(): Promise<void> {
  loading.value = true;
  lastError.value = "";
  try {
    await loadEnums();
    summaries.value = (await ClientSvc.ListClients()) ?? [];
  } catch (err) {
    lastError.value = backendErrMessage(err);
    summaries.value = [];
  } finally {
    loading.value = false;
  }
}

/** Load one client with its parsed document. Clears the selection on failure
 *  rather than leaving a stale client on screen under a new name. */
async function select(id: string): Promise<Result> {
  try {
    detail.value = await ClientSvc.GetClient(id);
    return ok();
  } catch (err) {
    detail.value = null;
    return failed(err);
  }
}

function clearSelection(): void {
  detail.value = null;
}

async function importSpec(
  name: string,
  base64Data: string,
): Promise<Result & { file: string; catalog: Catalog | null }> {
  try {
    const info = await ClientSvc.ImportSpec(name, base64Data);
    return { ...ok(), file: info.file, catalog: info.catalog };
  } catch (err) {
    return { ...failed(err), file: "", catalog: null };
  }
}

async function save(client: Connection): Promise<Result> {
  try {
    await ClientSvc.SaveClient(client);
    await refresh();
    await select(client.id);
    return ok();
  } catch (err) {
    return failed(err);
  }
}

async function remove(id: string): Promise<Result> {
  try {
    await ClientSvc.DeleteClient(id);
    detail.value = null;
    await refresh();
    return ok();
  } catch (err) {
    return failed(err);
  }
}

/** Validate a draft without saving, so the editor can show problems while the
 *  user is still typing. */
async function validate(client: Connection): Promise<ValidationError[]> {
  try {
    return (await ClientSvc.ValidateClient(client)) ?? [];
  } catch {
    return [];
  }
}

async function reloadSpecs(): Promise<Result> {
  try {
    await ClientSvc.ReloadSpecs();
    await refresh();
    return ok();
  } catch (err) {
    return failed(err);
  }
}

async function listItems(req: ListRequest): Promise<Result & { page: Page | null }> {
  try {
    return { ...ok(), page: await ClientSvc.ListItems(req) };
  } catch (err) {
    return { ...failed(err), page: null };
  }
}

async function fetchItem(req: FetchRequest): Promise<Result & { item: Item | null }> {
  try {
    return { ...ok(), item: await ClientSvc.FetchItem(req) };
  } catch (err) {
    return { ...failed(err), item: null };
  }
}

// FetchSnapshot returns the value the api-client field stores. An error yields
// no snapshot on purpose, so the caller keeps whatever is already on disk.
async function fetchSnapshot(
  req: FetchRequest,
): Promise<Result & { snapshot: Record<string, any> | null }> {
  try {
    return { ...ok(), snapshot: await ClientSvc.FetchSnapshot(req) };
  } catch (err) {
    return { ...failed(err), snapshot: null };
  }
}

async function setCredential(id: string, secret: string): Promise<Result> {
  try {
    await ClientSvc.SetCredential(id, secret);
    await select(id);
    return ok();
  } catch (err) {
    return failed(err);
  }
}

async function forgetCredential(id: string): Promise<Result> {
  try {
    await ClientSvc.ForgetCredential(id);
    await select(id);
    return ok();
  } catch (err) {
    return failed(err);
  }
}

export function useAPIClients() {
  return {
    summaries,
    detail,
    loading,
    lastError,
    dialects,
    keyStyles,
    paginationStyles,

    hasClients: computed(() => summaries.value.length > 0),
    selectedId: computed(() => detail.value?.client.id ?? ""),

    refresh,
    select,
    clearSelection,
    importSpec,
    save,
    remove,
    validate,
    reloadSpecs,
    listItems,
    fetchItem,
    fetchSnapshot,
    setCredential,
    forgetCredential,
  };
}
