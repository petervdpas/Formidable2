<script setup lang="ts">
import TextField from "./TextField.vue";
import { Service as DialogSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/dialog";
import { Service as SystemSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/system";
import { useI18n } from "vue-i18n";

// Readonly path field paired with a native folder picker. Free
// typing is intentionally disabled - the OS picker guarantees an
// absolute, existing folder, which is the only thing config and
// git-root code paths can rely on cheaply. Cancellation (empty
// returned path) is treated as a no-op.
//
// Picked paths are passed through SystemSvc.MakeAppRootRelative so
// folders under AppRoot collapse to "./<rel>" - keeping config
// values portable across machines that share the AppRoot
// convention. Outside-root paths round-trip absolute.
//
// Drop-in replacement for <TextField v-model="..."/> wherever the
// underlying value is a folder path.
const props = defineProps<{
  modelValue: string;
  placeholder?: string;
}>();
const emit = defineEmits<{ (e: "update:modelValue", v: string): void }>();

const { t } = useI18n();

async function browse() {
  const picked = await DialogSvc.ChooseDirectory();
  if (!picked) return;
  const display = await SystemSvc.MakeAppRootRelative(picked);
  emit("update:modelValue", display || picked);
}
</script>

<template>
  <div class="path-field">
    <TextField
      :model-value="props.modelValue"
      readonly
      :placeholder="props.placeholder"
    />
    <button
      type="button"
      class="tool-btn path-field-browse"
      :title="t('common.browse')"
      @click="browse"
    >
      …
    </button>
  </div>
</template>
