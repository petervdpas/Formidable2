import type { InjectionKey, Ref } from "vue";
import type { Facet } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type { FacetState } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";

/*
 * FacetContext is the bridge between the active workspace (StorageWorkspace
 * owns the open draft + the template's facets) and any inline virtual
 * facet field renderer placed inside the form. The renderer injects
 * this context, looks up its bound facet by key, and writes changes
 * through the same handler the corner FacetPicker uses, so both
 * setters stay in sync without a second source of truth.
 *
 * Workspaces that don't render facets (e.g. PluginsWorkspace using
 * FormFieldRow in isolation) simply don't provide this context;
 * FormFieldFacet then falls back to an inert hint.
 */
export interface FacetContext {
  facets: Ref<Facet[]>;
  state: Ref<{ [key: string]: FacetState | undefined }>;
  onChange: (key: string, state: FacetState) => void;
}

export const FACET_CONTEXT_KEY: InjectionKey<FacetContext> = Symbol("facetContext");
