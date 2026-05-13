<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { Service as TemplateSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  filename: string;
  active: boolean;
}>();

defineEmits<{
  (e: "pick", filename: string): void;
}>();

// Self-serving display name: each row loads its own template metadata
// and derives its display title (template.name, falling back to the
// stem of the filename). The parent calls refresh() on the matching
// ref after a save so editing a template's name updates the sidebar
// row without thrashing the rest of the list.
const display = ref<string>(stemOf(props.filename));

function stemOf(fn: string): string {
  return fn.replace(/\.yaml$/, "");
}

async function loadDisplay(): Promise<void> {
  if (!props.filename) {
    display.value = "";
    return;
  }
  try {
    const tpl = await TemplateSvc.LoadTemplate(props.filename);
    const name = tpl?.name?.trim();
    display.value = name && name.length > 0 ? name : stemOf(props.filename);
  } catch {
    display.value = stemOf(props.filename);
  }
}

onMounted(loadDisplay);
watch(() => props.filename, loadDisplay);

defineExpose({ refresh: loadDisplay });
</script>

<template>
  <li
    :class="['sidebar-row', 'sidebar-row--stack', { active: props.active }]"
    :data-filename="props.filename"
    @click="$emit('pick', props.filename)"
  >
    <span class="template-display">{{ display }}</span>
    <span class="template-meta">
      <span class="badge small template-filename">{{ props.filename }}</span>
    </span>
  </li>
</template>
