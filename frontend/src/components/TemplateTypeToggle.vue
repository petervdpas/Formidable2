<script setup lang="ts">
// TemplateTypeToggle - the one place a template picks its record "type":
// a plain collection, a presentation deck, or a plan board. Each mode
// depends on a prerequisite (a field type, or the other mode being off),
// so instead of showing every toggle greyed out with a "needs X" hint,
// this widget shows a toggle ONLY when it is actionable: either already
// on (so it can be turned off) or its prerequisite is met (so it can be
// turned on). Nothing renders when the form has none of the required
// fields yet.
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { FormSwitchRow } from "./fields";
import type { Field } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{ fields: Field[] }>();

const enableCollection = defineModel<boolean>("enableCollection", { default: false });
const presentation = defineModel<boolean>("presentation", { default: false });
const projectMode = defineModel<boolean>("projectMode", { default: false });

const { t } = useI18n();

const hasField = (type: string) => props.fields.some((f) => f.type === type);
const hasGuid = computed(() => hasField("guid"));
const hasSequence = computed(() => hasField("sequence"));
const hasProject = computed(() => hasField("project"));

// A toggle is shown only when actionable. Presentation and Project are the two
// mutually-exclusive record models: turning one on hides the other (its
// prerequisite reads "off"), so the old "both on" conflict can't be reached.
const showCollection = computed(() => enableCollection.value || hasGuid.value);
const showPresentation = computed(
  () => presentation.value || (hasSequence.value && !projectMode.value),
);
const showProject = computed(
  () => projectMode.value || (hasProject.value && !presentation.value),
);

</script>

<!--
  No wrapping element: .form-section is a grid and each .form-switch-row uses
  grid-template-columns: subgrid, which only inherits the section's columns when
  the row is a DIRECT child. This component is a fragment so its rows splice
  straight into the section grid, exactly like the old inline switch rows.
-->
<template>
  <FormSwitchRow
    v-if="showCollection"
    v-model="enableCollection"
    :label="t('workspace.templates.setup.enable_collection')"
    :on-label="t('common.on')"
    :off-label="t('common.off')"
  />
  <FormSwitchRow
    v-if="showPresentation"
    v-model="presentation"
    :label="t('workspace.templates.setup.presentation')"
    :description="t('workspace.templates.setup.presentation_desc')"
    :on-label="t('common.on')"
    :off-label="t('common.off')"
  />
  <FormSwitchRow
    v-if="showProject"
    v-model="projectMode"
    :label="t('workspace.templates.setup.project_mode')"
    :description="t('workspace.templates.setup.project_mode_desc')"
    :on-label="t('common.on')"
    :off-label="t('common.off')"
  />
</template>
