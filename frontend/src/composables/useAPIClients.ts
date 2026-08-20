import { computed, ref } from "vue";
import {
  Service as ClientSvc,
  type Catalog,
  type ClientDetail,
  type Connection,
  type FetchRequest,
  type Item,
  type ListRequest,
  type Detection,
  type MethodDescriptor,
  type OperationInfo,
  type Page,
  type SpecSource,
  type TryForm,
  type TryRequest,
  type TryResult,
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
const itemsModes = ref<string[]>([]);
const shapes = ref<string[]>([]);
const methods = ref<MethodDescriptor[]>([]);


type Result = { ok: boolean; message: string };

const ok = (): Result => ({ ok: true, message: "" });
const failed = (err: unknown): Result => ({ ok: false, message: backendErrMessage(err) });

async function loadEnums(): Promise<void> {
  if (dialects.value.length > 0) return;
  dialects.value = (await ClientSvc.ListDialects()) ?? [];
  keyStyles.value = (await ClientSvc.ListKeyStyles()) ?? [];
  paginationStyles.value = (await ClientSvc.ListPaginationStyles()) ?? [];
  itemsModes.value = (await ClientSvc.ListItemsModes()) ?? [];
  shapes.value = (await ClientSvc.ListShapes()) ?? [];
  methods.value = (await ClientSvc.ListMethods()) ?? [];
  applyMethodPalette(methods.value);
}

// The method badge colours come from Go with the method list. Writing them onto
// the root here keeps the stylesheet free of a second copy that would drift the
// moment the backend learns a new method.
function applyMethodPalette(descriptors: MethodDescriptor[]): void {
  const root = document.documentElement;
  for (const d of descriptors) {
    const token = d.method.toLowerCase();
    root.style.setProperty(`--http-method-${token}-bg`, d.palette.bg);
    root.style.setProperty(`--http-method-${token}-border`, d.palette.border);
    root.style.setProperty(`--http-method-${token}-text`, d.palette.text);
  }
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

// Read the document and propose bindings. The draft goes over rather than the
// id, so an unsaved edit counts: whatever it already binds is left out.
async function detectResources(
  client: Connection,
): Promise<Result & { detection: Detection | null }> {
  try {
    return { ...ok(), detection: await ClientSvc.DetectResources(client) };
  } catch (err) {
    return { ...failed(err), detection: null };
  }
}

// Every operation the document declares, annotated with what it returns and
// which resources bind it.
async function listOperations(
  client: Connection,
): Promise<Result & { operations: OperationInfo[] }> {
  try {
    return { ...ok(), operations: (await ClientSvc.ListOperations(client)) ?? [] };
  } catch (err) {
    return { ...failed(err), operations: [] };
  }
}

// The page that renders this client's document with Swagger UI. Loopback HTTP
// rather than a blob: the renderer resolves refs by fetching the document, and
// only a real origin makes that fetch succeed.
async function docsURL(client: Connection): Promise<Result & { url: string }> {
  try {
    return { ...ok(), url: await ClientSvc.DocsURL(client) };
  } catch (err) {
    return { ...failed(err), url: "" };
  }
}

// The uploaded document itself. Reading the source is half of authoring a
// binding, so it does not need the client saved first.
async function specSource(
  client: Connection,
): Promise<Result & { source: SpecSource | null }> {
  try {
    return { ...ok(), source: await ClientSvc.SpecSource(client) };
  } catch (err) {
    return { ...failed(err), source: null };
  }
}

// What running an operation would need, with whatever a resource already fixes
// filled in.
async function tryForm(
  client: Connection,
  operation: string,
): Promise<Result & { form: TryForm | null }> {
  try {
    return { ...ok(), form: await ClientSvc.TryOperationForm(client, operation) };
  } catch (err) {
    return { ...failed(err), form: null };
  }
}

// A failing status comes back as a result, not an error: seeing the remote's
// own 404 body is the point. Only a refusal before the call throws.
async function tryOperation(
  req: TryRequest,
): Promise<Result & { result: TryResult | null }> {
  try {
    return { ...ok(), result: await ClientSvc.TryOperation(req) };
  } catch (err) {
    return { ...failed(err), result: null };
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
    itemsModes,
    shapes,
    methods,

    hasClients: computed(() => summaries.value.length > 0),
    selectedId: computed(() => detail.value?.client.id ?? ""),

    refresh,
    select,
    clearSelection,
    importSpec,
    save,
    remove,
    validate,
    detectResources,
    listOperations,
    specSource,
    docsURL,
    tryForm,
    tryOperation,
    reloadSpecs,
    listItems,
    fetchItem,
    fetchSnapshot,
    setCredential,
    forgetCredential,
  };
}
