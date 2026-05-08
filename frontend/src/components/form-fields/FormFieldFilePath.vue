<script setup lang="ts">
import { computed } from "vue";
import { TextField } from "../fields";
import {
  Service as DialogSvc,
  FileFilter,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/dialog";
import { Service as SystemSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/system";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// Path-shaped string input + a Browse button that opens the OS
// file picker. field.options carries extension globs that become
// FileFilter entries in the picker. Each option is either a plain
// string ("*.json") or an object ({ label, pattern }).
//
// All values are normalized to absolute paths:
//   - Picker output is already absolute on every platform.
//   - Hand-typed input is coerced via SystemSvc.ResolveAbsolutePath
//     on focusout (`~`/`~/sub` → home, relative → cwd-resolved,
//     empty → empty so a freshly cleared field stays empty).
// The Browse button stays enabled even when readonly — read-only on
// path fields blocks free-typing but should not block picking a new
// path through the OS dialog.
const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const value = computed<string>({
  get: () => (props.modelValue == null ? "" : String(props.modelValue)),
  set: (v) => emit("update:modelValue", v),
});

const filters = computed<FileFilter[]>(() => {
  const opts = (props.field.options ?? []) as unknown[];
  const out: FileFilter[] = [];
  for (const o of opts) {
    if (typeof o === "string") {
      const pat = o.trim();
      if (pat) out.push(new FileFilter({ displayName: pat, pattern: pat }));
      continue;
    }
    if (o && typeof o === "object") {
      const obj = o as Record<string, unknown>;
      const pat = String(obj.pattern ?? obj.value ?? "").trim();
      const lbl = String(obj.label ?? obj.displayName ?? pat).trim();
      if (pat) out.push(new FileFilter({ displayName: lbl, pattern: pat }));
    }
  }
  return out;
});

async function coerceAbsolute() {
  if (!value.value) return;
  try {
    const abs = await SystemSvc.ResolveAbsolutePath(value.value);
    if (abs && abs !== value.value) value.value = abs;
  } catch {
    // Resolve failures (e.g. no home dir on a stripped runtime)
    // leave the typed value alone — better than silently clearing.
  }
}

async function browse() {
  try {
    const picked = await DialogSvc.ChooseFile(filters.value);
    if (picked) {
      value.value = picked;
      // Picker returns absolute on every platform; coerce anyway so
      // any platform that ever returns a relative path stays normalized.
      await coerceAbsolute();
    }
  } catch {
    // Picker errors (rare — e.g. no display server) leave the field
    // untouched. The user can still type the path manually.
  }
}
</script>

<template>
  <div class="path-field" @focusout="coerceAbsolute">
    <TextField v-model="value" :readonly="field.readonly" />
    <button
      type="button"
      class="tool-btn path-field-browse"
      @click="browse"
    >
      …
    </button>
  </div>
</template>
