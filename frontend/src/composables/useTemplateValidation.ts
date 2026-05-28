import { computed, onScopeDispose, ref, watch } from "vue";
import { Service as RenderSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/render";
import type {
  Diagnostic,
  ValidationReport,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/render/models";

// useTemplateValidation
// Live-validates a Handlebars/Markdown template source via the render
// module. The caller passes a getter (so the composable stays
// reactivity-source agnostic), and gets back:
//
//   - report:              the latest ValidationReport (null until first run)
//   - errorDiagnostic:     the first error-severity Diagnostic (or null)
//   - warningDiagnostics:  every warning Diagnostic
//   - isOK:                true iff the latest run returned ok with no diagnostics
//   - pending:             true while a debounce timer is still pending
//
// A 300ms debounce smooths over keystroke storms; the timer is cleared
// when the consuming scope is disposed so this is safe inside any
// component or nested setup.
export function useTemplateValidation(
  source: () => string | undefined | null,
  options: { debounceMs?: number } = {},
) {
  const debounceMs = options.debounceMs ?? 300;

  const report = ref<ValidationReport | null>(null);
  const pending = ref(false);
  let timer: ReturnType<typeof setTimeout> | null = null;

  async function runNow(src: string) {
    if (!src || !src.trim()) {
      report.value = { ok: true, diagnostics: [] } as ValidationReport;
      return;
    }
    try {
      report.value = await RenderSvc.ValidateMarkdownTemplate(src);
    } catch {
      report.value = null;
    }
  }

  watch(
    source,
    (src) => {
      pending.value = true;
      if (timer) clearTimeout(timer);
      timer = setTimeout(async () => {
        await runNow(src ?? "");
        pending.value = false;
      }, debounceMs);
    },
    { immediate: true },
  );

  onScopeDispose(() => {
    if (timer) clearTimeout(timer);
  });

  const errorDiagnostic = computed<Diagnostic | null>(() => {
    const diags = report.value?.diagnostics ?? [];
    return diags.find((d) => d.severity === "error") ?? null;
  });

  const warningDiagnostics = computed<Diagnostic[]>(() => {
    const diags = report.value?.diagnostics ?? [];
    return diags.filter((d) => d.severity === "warning");
  });

  const isOK = computed(
    () =>
      report.value?.ok === true &&
      (report.value?.diagnostics?.length ?? 0) === 0,
  );

  return {
    report,
    errorDiagnostic,
    warningDiagnostics,
    isOK,
    pending,
  };
}
