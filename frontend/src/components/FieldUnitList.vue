<script setup lang="ts">
import draggable from "vuedraggable";
import Badge from "./Badge.vue";
import FieldScopeBadge from "./FieldScopeBadge.vue";
import type {
  FieldUnit,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// Recursive tree-aware list for the template field editor. The
// LOOPSTART/LOOPSTOP pair (and everything between them) is one unit
// at this level — its interior is a nested list rooted inside the
// unit. That makes it impossible to drop a sibling field between
// loopstart and loopstop by mistake. Depth bumps the visible indent
// so nesting is obvious.
//
// Edit/Delete events carry the FieldUnit *reference* — the parent
// resolves identity by walking the tree for that exact object, never
// by matching field content. That keeps each unit isolated even when
// two fields would otherwise look identical.

defineProps<{
  units: FieldUnit[];
  depth?: number;
}>();

const emit = defineEmits<{
  (e: "change"): void;
  (e: "edit-unit", u: FieldUnit): void;
  (e: "delete-unit", u: FieldUnit): void;
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
            @click="emit('edit-unit', u)"
          >Edit</button>
          <button
            type="button"
            class="field-action-btn delete"
            @click="emit('delete-unit', u)"
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
              @click="emit('edit-unit', u)"
            >Edit</button>
            <button
              type="button"
              class="field-action-btn delete"
              @click="emit('delete-unit', u)"
            >Delete</button>
          </div>
        </div>

        <FieldUnitList
          :units="u.items ?? []"
          :depth="(depth ?? 0) + 1"
          @change="emit('change')"
          @edit-unit="(child) => emit('edit-unit', child)"
          @delete-unit="(child) => emit('delete-unit', child)"
        />

        <div class="field-row field-loop-footer" data-type="loopstop">
          <span class="field-row-label">{{ u.stop.label || u.stop.key }}</span>
          <span class="field-row-type">(LOOPSTOP)</span>
        </div>
      </li>
    </template>
  </draggable>
</template>
