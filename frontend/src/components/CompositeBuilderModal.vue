<script setup lang="ts">
/*
 * CompositeBuilderModal - authors one composite statistical object (a hop
 * route): a parent distribution whose branches drill into other objects.
 * It is driven entirely by Stat.CompositeOptions, which the backend computes
 * from the template's objects (a child is eligible for a branch only if it
 * already filters the parent's base to that branch value). The author can
 * therefore only wire links the engine accepts - backend steers, the
 * structure is the gate. The output is a Statistic carrying a composite spec.
 */
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import { SelectField, TextField } from "./fields";
import { Service as StatSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/stat";
import type { CompositeOption } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/stat/models";
import type { Statistic } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  open: boolean;
  template: string;
  /** Existing objects, for friendly parent/child labels. */
  statistics: Statistic[];
  /** The composite being edited, or null to compose a new one. */
  initial: Statistic | null;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "apply", stat: Statistic): void;
}>();

const { t } = useI18n();

const name = ref("");
const label = ref("");
const parent = ref("");
const childByBranch = ref<Record<string, string>>({});
const options = ref<CompositeOption[]>([]);

function labelFor(objName: string): string {
  return props.statistics.find((s) => s.name === objName)?.label || objName;
}

const parentOptions = computed(() =>
  options.value.map((o) => ({ value: o.parent, label: labelFor(o.parent) })),
);

const branches = computed(() => {
  const o = options.value.find((x) => x.parent === parent.value);
  return o?.edges ?? [];
});

function childOptionsFor(branch: string) {
  const e = branches.value.find((b) => b.branch === branch);
  const opts = [{ value: "", label: t("workspace.templates.composite_builder.none") }];
  for (const c of e?.children ?? []) opts.push({ value: c, label: labelFor(c) });
  return opts;
}

function setParent(p: string) {
  parent.value = p;
  childByBranch.value = {}; // a new parent has its own branches
}
function setChild(branch: string, child: string) {
  childByBranch.value = { ...childByBranch.value, [branch]: child };
}

const edges = computed(() =>
  branches.value
    .map((b) => ({ branch: b.branch, child: childByBranch.value[b.branch] || "" }))
    .filter((e) => e.child !== ""),
);

const hasOptions = computed(() => options.value.length > 0);
const canApply = computed(
  () => name.value.trim() !== "" && parent.value !== "" && edges.value.length > 0,
);

watch(
  () => props.open,
  async (open) => {
    if (!open) return;
    options.value = await StatSvc.CompositeOptions(props.template);
    if (props.initial?.composite) {
      name.value = props.initial.name;
      label.value = props.initial.label || "";
      parent.value = props.initial.composite.parent;
      const map: Record<string, string> = {};
      for (const e of props.initial.composite.edges ?? []) map[e.branch] = e.child;
      childByBranch.value = map;
    } else {
      name.value = "";
      label.value = "";
      parent.value = "";
      childByBranch.value = {};
    }
  },
  { immediate: true },
);

function onApply() {
  emit("apply", {
    name: name.value.trim(),
    label: label.value.trim(),
    dsl: "",
    composite: { parent: parent.value, edges: edges.value },
  } as unknown as Statistic);
}
</script>

<template>
  <Modal
    :open="open"
    :title="t('workspace.templates.composite_builder.title')"
    width="640px"
    scroll
    @close="emit('close')"
  >
    <p class="muted small stat-builder-hint">
      {{ t('workspace.templates.composite_builder.intro') }}
    </p>
    <p v-if="!hasOptions" class="muted small stat-builder-empty">
      {{ t('workspace.templates.composite_builder.no_options') }}
    </p>

    <div v-else class="stat-builder-form">
      <div class="stat-builder-ident">
        <label class="stat-builder-field">
          <span class="stat-builder-field-label">{{ t('workspace.templates.stat_builder.name') }}</span>
          <TextField v-model="name" placeholder="in-use-by-app" />
        </label>
        <label class="stat-builder-field">
          <span class="stat-builder-field-label">{{ t('workspace.templates.stat_builder.label') }}</span>
          <TextField v-model="label" />
        </label>
      </div>

      <div class="stat-builder-blockedit">
        <div class="stat-block-group">{{ t('workspace.templates.composite_builder.parent') }}</div>
        <SelectField
          :model-value="parent"
          :options="parentOptions"
          :placeholder="t('workspace.templates.composite_builder.pick_parent')"
          @update:model-value="setParent"
        />

        <template v-if="parent">
          <div class="stat-block-group">{{ t('workspace.templates.composite_builder.branches') }}</div>
          <p class="muted small stat-builder-hint">
            {{ t('workspace.templates.composite_builder.branches_hint') }}
          </p>
          <div class="stat-composite-branches">
            <div v-for="b in branches" :key="b.branch" class="stat-composite-row">
              <span
                :class="[
                  'stat-block-item',
                  childByBranch[b.branch] ? 'is-dimension' : 'is-filter',
                  'stat-composite-branch',
                ]"
              >{{ b.branch }}</span>
              <SelectField
                :model-value="childByBranch[b.branch] || ''"
                :options="childOptionsFor(b.branch)"
                class="stat-composite-childsel"
                @update:model-value="(v: string) => setChild(b.branch, v)"
              />
            </div>
          </div>
        </template>
      </div>
    </div>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('close')">
        {{ t('common.cancel') }}
      </button>
      <button class="tool-btn primary" type="button" :disabled="!canApply" @click="onApply">
        {{ t('workspace.templates.stat_builder.apply') }}
      </button>
    </template>
  </Modal>
</template>
