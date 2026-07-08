import { Call } from "@wailsio/runtime";

// Fully-qualified name Wails resolves ByName calls against: package path + type
// + method. Using ByName lets this small app skip binding generation entirely.
const SVC = "github.com/petervdpas/formidable2/internal/modules/viewer.Service";

export interface Config {
  language: string;
  theme: string;
  default_zoom: number;
  remember_size: boolean;
  window_width: number;
  window_height: number;
  recent_bundles: string[];
  serve_http: boolean;
  http_port: number;
}

export interface BundleInfo {
  loaded: boolean;
  name: string;
  title: string;
  description: string;
  author: string;
  created: string;
  encrypted: boolean;
}

export interface OpenResult {
  info: BundleInfo;
  needsPassword: boolean;
  wrongPassword: boolean;
  path: string;
}

export interface RecentInfo {
  path: string;
  name: string;
  exists: boolean;
}

export interface ServerStatus {
  running: boolean;
  port: number;
  urls: string[];
}

function call<T>(method: string, ...args: unknown[]): Promise<T> {
  return Call.ByName(`${SVC}.${method}`, ...args) as Promise<T>;
}

export const api = {
  getConfig: () => call<Config>("GetConfig"),
  setConfig: (c: Config) => call<Config>("SetConfig", c),
  languages: () => call<string[]>("Languages"),
  effectiveLanguage: () => call<string>("EffectiveLanguage"),
  messages: (lang: string) => call<Record<string, string>>("Messages", lang),
  recents: () => call<RecentInfo[]>("Recents"),
  openDialog: () => call<OpenResult>("OpenDialog"),
  openPath: (p: string, password: string) => call<OpenResult>("OpenPath", p, password),
  openBytes: (name: string, dataB64: string, password: string) =>
    call<OpenResult>("OpenBytes", name, dataB64, password),
  takePendingOpen: () => call<string>("TakePendingOpen"),
  current: () => call<BundleInfo>("Current"),
  serverStatus: () => call<ServerStatus>("ServerStatus"),
  bundleURL: () => call<string>("BundleURL"),
};

// Event emitted by the Go side whenever the open bundle changes (drop, dialog,
// or recent). The shell listens so it can refresh state and reload the frame.
export const BundleChangedEvent = "viewer:bundle-changed";
