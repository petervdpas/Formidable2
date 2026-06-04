<script setup lang="ts">
import { computed } from "vue";
import FormFieldRow from "./FormFieldRow.vue";
import FormLoop from "./FormLoop.vue";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type { LoopGroup } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/form";

// FormLoopFields walks one slice of template.fields[] and dispatches
// each field to either:
//   - FormLoop (when a loopstart's index matches a LoopGroup) - skips
//     the inner fields up to the matching loopstop
//   - FormFieldRow (regular field - guid + loopstop are hidden)
//
// `startOffset` is where this slice begins in the ORIGINAL template
// fields array; loop_groups indices are absolute, so we add the
// offset when matching against fields' positions inside the slice.

const props = defineProps<{
  fields: Field[];
  startOffset: number;
  values: Record<string, unknown>;
  loopGroups: LoopGroup[];
}>();

// formula is a virtual controller field: it carries no value of its own and is
// invisible in the rendered form. Its effect (and the live Compute button) lives
// on its target field instead.
const HIDDEN: Set<string> = new Set(["guid", "loopstop", "formula"]);

// Build [absoluteIndex → group] for O(1) lookup while walking.
const groupByStart = computed(() => {
  const map = new Map<number, LoopGroup>();
  for (const g of props.loopGroups) map.set(g.start_index, g);
  return map;
});

// Pre-walk the slice into a list of render entries - either a
// regular field (with its index in the slice) or a loop block.
type Entry =
  | { kind: "field"; field: Field; key: string }
  | { kind: "loop"; group: LoopGroup; field: Field; innerStart: number; innerStop: number };

const entries = computed<Entry[]>(() => {
  const out: Entry[] = [];
  let i = 0;
  while (i < props.fields.length) {
    const f = props.fields[i];
    const abs = props.startOffset + i;
    if (f.type === "loopstart") {
      const group = groupByStart.value.get(abs);
      if (group) {
        out.push({
          kind: "loop",
          group,
          field: f,
          innerStart: i + 1,
          innerStop: group.stop_index - props.startOffset,
        });
        // Skip past the matching loopstop.
        i = group.stop_index - props.startOffset + 1;
        continue;
      }
      // Unpaired (validator catches this) - render as hidden, advance.
      i++;
      continue;
    }
    out.push({ kind: "field", field: f, key: `${abs}:${f.key}` });
    i++;
  }
  return out;
});

function setFieldValue(key: string, v: unknown) {
  props.values[key] = v;
}
function setLoopValue(key: string, v: unknown[]) {
  props.values[key] = v;
}
function loopArray(key: string): unknown[] {
  const v = props.values[key];
  return Array.isArray(v) ? v : [];
}
</script>

<template>
  <template v-for="entry in entries" :key="entry.kind === 'loop' ? `loop:${entry.group.key}:${entry.group.start_index}` : entry.key">
    <FormLoop
      v-if="entry.kind === 'loop'"
      :field="entry.field"
      :group="entry.group"
      :inner-fields="fields.slice(entry.innerStart, entry.innerStop)"
      :inner-start-offset="startOffset + entry.innerStart"
      :loop-groups="loopGroups"
      :model-value="loopArray(entry.field.key)"
      @update:model-value="(v: unknown[]) => setLoopValue(entry.field.key, v)"
    />
    <FormFieldRow
      v-else-if="!HIDDEN.has(entry.field.type)"
      :field="entry.field"
      :model-value="values[entry.field.key]"
      @update:model-value="(v: unknown) => setFieldValue(entry.field.key, v)"
    />
  </template>
</template>
