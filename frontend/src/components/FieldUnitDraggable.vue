<script setup lang="ts">
import draggable from "vuedraggable";
import Badge from "./Badge.vue";
import FieldScopeBadge from "./FieldScopeBadge.vue";
import type {
  Field,
  FieldUnit,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// Recursive tree-aware draggable for the template field list. The
// LOOPSTART/LOOPSTOP pair (and everything between them) is one unit
// at this level — its interior is a nested draggable rooted inside
// the unit. That makes it impossible to drop a sibling field
// between loopstart and loopstop by mistake. Depth bumps the visible
// indent so nesting is obvious.

defineProps<{
  units: FieldUnit[];
  depth?: number;
}>();

const emit = defineEmits<{
  (e: "change"): void;
  (e: "edit-field", f: Field): void;
  (e: "delete-field", f: Field): void;
}>();

function unitKey(u: FieldUnit): string {
  if (u.kind === "loop") return `loop:${u.start?.key ?? ""}`;
  const f = u.field;
  return `field:${(f?.type ?? "").toLowerCase()}:${f?.key ?? ""}`;
}
</script>

<template>
  <draggable
    :list="units"
    tag="ul"
    class="field-rows"
    handle=".dnd-handle"
    :animation="150"
    ghost-class="dnd-ghost"
    chosen-class="dnd-chosen"
    drag-class="dnd-drag"
    :item-key="unitKey"
    group="template-fields"
    @end="emit('change')"
    @add="emit('change')"
    @remove="emit('change')"
  >
    <template #item="{ element: u }">
      <li
        v-if="u.kind === 'field' && u.field"
        class="field-row"
        :data-type="u.field.type"
      >
        <span class="dnd-handle" aria-hidden="true">☰</span>
        <span class="field-row-label">{{ u.field.label || u.field.key }}</span>
        <span class="field-row-type">({{ (u.field.type || '').toUpperCase() }})</span>
        <Badge v-if="u.field.primary_key" variant="ok" class="small">PRIMARY</Badge>
        <span class="field-row-spacer"></span>
        <div class="field-row-actions">
          <FieldScopeBadge :level="u.field.level_scope" />
          <button
            type="button"
            class="field-action-btn edit"
            @click="emit('edit-field', u.field)"
          >Edit</button>
          <button
            type="button"
            class="field-action-btn delete"
            @click="emit('delete-field', u.field)"
          >Delete</button>
        </div>
      </li>

      <li
        v-else-if="u.kind === 'loop' && u.start && u.stop"
        class="field-loop-group"
        :data-depth="depth ?? 0"
      >
        <div class="field-row field-loop-header" data-type="loopstart">
          <span class="dnd-handle" aria-hidden="true">☰</span>
          <span class="field-row-label">{{ u.start.label || u.start.key }}</span>
          <span class="field-row-type">(LOOPSTART)</span>
          <span class="field-row-spacer"></span>
          <div class="field-row-actions">
            <FieldScopeBadge :level="u.start.level_scope" />
            <button
              type="button"
              class="field-action-btn edit"
              @click="emit('edit-field', u.start)"
            >Edit</button>
            <button
              type="button"
              class="field-action-btn delete"
              @click="emit('delete-field', u.start)"
            >Delete</button>
          </div>
        </div>

        <FieldUnitDraggable
          :units="u.items ?? []"
          :depth="(depth ?? 0) + 1"
          @change="emit('change')"
          @edit-field="(f) => emit('edit-field', f)"
          @delete-field="(f) => emit('delete-field', f)"
        />

        <div class="field-row field-loop-footer" data-type="loopstop">
          <span class="field-row-label">{{ u.stop.label || u.stop.key }}</span>
          <span class="field-row-type">(LOOPSTOP)</span>
        </div>
      </li>
    </template>
  </draggable>
</template>
