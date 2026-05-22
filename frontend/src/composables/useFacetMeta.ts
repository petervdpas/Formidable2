import { ref, computed } from "vue";
import * as TemplateSvc from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template/service";
import type {
  FacetIconSpec,
  FacetMeta,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template/models";

// Module-scope singleton: every consumer shares one snapshot of the
// backend's facet contract (limits + palettes + regex patterns). The
// frontend never mirrors these constants - backend is authoritative
// per project rule [[feedback_backend_owns_data]].
const meta = ref<FacetMeta | null>(null);
const loading = ref(false);
let inflight: Promise<void> | null = null;

async function load() {
  if (meta.value || inflight) return inflight ?? Promise.resolve();
  loading.value = true;
  inflight = (async () => {
    try {
      meta.value = await TemplateSvc.FacetMeta();
    } finally {
      loading.value = false;
      inflight = null;
    }
  })();
  return inflight;
}

// Sensible fallbacks while the backend snapshot is in-flight. They
// match the current backend defaults so a render in the millisecond
// before the fetch resolves doesn't crash; the values are replaced
// the moment FacetMeta() returns.
const FALLBACK_COLORS: string[] = [];
const FALLBACK_ICONS: string[] = [];
const FALLBACK_ICON_SVGS: Record<string, FacetIconSpec> = {};

export function useFacetMeta() {
  void load();
  const maxFacets = computed(() => meta.value?.max_facets ?? 0);
  const maxOptionsPerFacet = computed(
    () => meta.value?.max_options_per_facet ?? 0,
  );
  const colors = computed(() => meta.value?.colors ?? FALLBACK_COLORS);
  const icons = computed(() => meta.value?.icons ?? FALLBACK_ICONS);
  const iconSvgs = computed<Record<string, FacetIconSpec>>(() => {
    const raw = meta.value?.icon_svgs;
    if (!raw) return FALLBACK_ICON_SVGS;
    // Wails' generated `{ [_ in string]?: FacetIconSpec }` shape isn't a
    // plain object lookup - normalise to one once so callers can index
    // by string without optional-chaining the result.
    const out: Record<string, FacetIconSpec> = {};
    for (const [k, v] of Object.entries(raw)) {
      if (v) out[k] = v;
    }
    return out;
  });
  const keyRegex = computed(() =>
    meta.value?.key_pattern ? new RegExp(meta.value.key_pattern) : /^$/,
  );
  const labelRegex = computed(() =>
    meta.value?.label_pattern ? new RegExp(meta.value.label_pattern) : /^$/,
  );
  return {
    ready: computed(() => meta.value !== null),
    loading: computed(() => loading.value),
    maxFacets,
    maxOptionsPerFacet,
    colors,
    icons,
    iconSvgs,
    keyRegex,
    labelRegex,
    reload: load,
  };
}
