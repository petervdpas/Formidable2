import type { InjectionKey, Ref } from "vue";

/*
 * FormValues is the bridge that lets an inline virtual field read sibling
 * data values from the active draft. The workspace owning the open form
 * (StorageWorkspace) provides the live values map; a virtual field renderer
 * that needs to project another field's value (e.g. the formula field showing
 * its target) injects it.
 *
 * Workspaces rendering form rows in isolation (e.g. PluginsWorkspace) simply
 * don't provide it; the consumer then falls back to an inert hint.
 */
export const FORM_VALUES_KEY: InjectionKey<Ref<Record<string, unknown>>> =
  Symbol("formValues");
