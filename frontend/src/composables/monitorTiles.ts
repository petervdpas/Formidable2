import * as MonitorSvc from "../../bindings/github.com/petervdpas/formidable2/internal/modules/monitor/service";
import * as JournalSvc from "../../bindings/github.com/petervdpas/formidable2/internal/modules/journal/service";
import {
  Aggregator,
  Query,
  Result,
  Series,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/monitor/models";

// MonitorTile is the per-card descriptor the MonitorBoard renders.
// `query` and `fetch` are mutually exclusive — use the first for tiles
// fed by a registered Source (most cases); use the second for tiles
// driven by a different Wails service (e.g. journal pending counts,
// which is a snapshot, not a Source).
export type MonitorTileChart = "timeseries" | "bars";

export interface MonitorTile {
  id: string;
  title: string;
  chart: MonitorTileChart;
  description?: string;
  query?: Query;
  fetch?: () => Promise<Result>;
}

// Build the v1 tile list. Hardcoded for now — when the user-DSL lands,
// this composable becomes a thin layer over a backend-supplied list.
//
// Important: the frontend uses Wails services directly (no /api/monitor
// HTTP). The Monitoring page works regardless of whether the loopback
// internal server is on.
export function defaultMonitorTiles(): MonitorTile[] {
  const now = new Date();
  const dayAgo = new Date(now.getTime() - 24 * 60 * 60 * 1000);

  return [
    {
      id: "mutations-24h",
      title: "Mutations (last 24h)",
      chart: "timeseries",
      description: "Create / update / delete events from the journal, hourly bins.",
      query: new Query({
        source: "journal",
        from: dayAgo,
        to: now,
        // Filter: just mutation ops — sync/baseline aren't churn signal.
        filter: { },
        group_by: ["op"],
        bin: "1h",
        agg: Aggregator.AggCount,
      }),
    },
    {
      id: "pending-per-backend",
      title: "Pending per backend",
      chart: "bars",
      description: "Files dirty since last sync, per backend.",
      // Pending is a snapshot, not a journal-event projection. Use the
      // dedicated JournalSvc.Pending() Wails call and stitch a Result
      // shaped like the chart expects.
      fetch: async (): Promise<Result> => {
        const backends = ["git", "gigot"];
        const series: Series[] = [];
        for (const backend of backends) {
          const pr = await JournalSvc.Pending(backend);
          series.push(
            new Series({
              key: { backend },
              total: pr?.count ?? 0,
            })
          );
        }
        return new Result({ series });
      },
    },
  ];
}

// runTile dispatches based on tile shape. Used by MonitorTile.vue so
// the component code doesn't repeat the if/else.
export async function runTile(tile: MonitorTile): Promise<Result> {
  if (tile.fetch) return tile.fetch();
  if (tile.query) {
    const res = await MonitorSvc.Run(tile.query);
    return res ?? new Result({ series: [] });
  }
  return new Result({ series: [] });
}

// ─── Tile-order persistence ──────────────────────────────────────────
// Persistence uses localStorage as a v1 — survives reloads, doesn't
// require a backend round-trip on every drag. When user-customisable
// tile sets land, this can swap to a Wails-backed config field
// without changing the component contract (the workspace still v-models
// the array; only loadTileOrder/saveTileOrder change).

const ORDER_STORAGE_KEY = "formidable.monitor.tile-order";

// loadTileOrder reads the saved id-order from localStorage. Unknown
// or missing ids in the stored order are dropped; tiles in `available`
// that aren't in the stored order are appended at the end. That way:
//   - an upgrade that adds a new default tile → it appears at the end
//   - removing a default tile → its stored id silently goes away
//   - a corrupted/empty store → default order is returned unchanged
export function loadTileOrder(available: MonitorTile[]): MonitorTile[] {
  let order: string[] = [];
  try {
    const raw = localStorage.getItem(ORDER_STORAGE_KEY);
    if (raw) {
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) {
        order = parsed.filter((x): x is string => typeof x === "string");
      }
    }
  } catch {
    // ignore — localStorage unavailable or quota issue
  }
  const byId = new Map(available.map((t) => [t.id, t]));
  const out: MonitorTile[] = [];
  const seen = new Set<string>();
  for (const id of order) {
    const t = byId.get(id);
    if (t && !seen.has(id)) {
      out.push(t);
      seen.add(id);
    }
  }
  for (const t of available) {
    if (!seen.has(t.id)) out.push(t);
  }
  return out;
}

export function saveTileOrder(tiles: MonitorTile[]): void {
  try {
    const ids = tiles.map((t) => t.id);
    localStorage.setItem(ORDER_STORAGE_KEY, JSON.stringify(ids));
  } catch {
    // best-effort persistence; reorder still works in memory
  }
}
