<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "../../components/Modal.vue";
import { SwitchField } from "../../components/fields";
import type {
  Detection,
  Resource,
  ResourceDraft,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/connection";

const { t } = useI18n();

const props = defineProps<{
  open: boolean;
  detection: Detection | null;
  busy: boolean;
}>();

const emit = defineEmits<{
  (e: "apply", value: Resource[]): void;
  (e: "close"): void;
}>();

const drafts = computed<ResourceDraft[]>(() => props.detection?.drafts ?? []);

// Which proposals the author wants. Everything starts accepted: the whole
// point is to fill an empty Resources tab in one move, and unticking one is
// cheaper than ticking five.
const accepted = ref<boolean[]>([]);

watch(
  () => [props.open, props.detection] as const,
  ([open]) => {
    if (!open) return;
    accepted.value = drafts.value.map(() => true);
  },
  { immediate: true },
);

const acceptedCount = computed(() => accepted.value.filter(Boolean).length);

// Nothing proposed has two opposite causes, and the author has to be told
// which one applies: everything here is already bound, or this document
// describes no list of records at all.
const emptyReason = computed(() => {
  const d = props.detection;
  if (!d) return "";
  if (d.bound > 0) return t("apiclients.detect.empty_bound", [String(d.bound)]);
  if (d.no_collection > 0) {
    return t("apiclients.detect.empty_no_collection", [String(d.no_collection)]);
  }
  return t("apiclients.detect.empty");
});

// Guessed names the attributes the document could not state, so the label is
// the same wording as the field it belongs to in the resource editor.
const GUESS_LABEL_KEYS = {
  items_path: "apiclients.resource.items_path",
  id_path: "apiclients.resource.id_path",
  label_path: "apiclients.resource.label_path",
  search_param: "apiclients.resource.search_param",
  search_template: "apiclients.resource.search_template",
  select_param: "apiclients.resource.select_param",
  pagination: "apiclients.resource.pagination",
} as const;

type GuessKey = keyof typeof GUESS_LABEL_KEYS;

function guessLabel(attr: string): string {
  const key = GUESS_LABEL_KEYS[attr as GuessKey];
  return key ? t(key) : attr;
}

// Two different reasons a proposal has no id pointer, and calling them the
// same thing is wrong: a keyed entry is addressed by its key, while a plain
// value is its own id.
function idOrigin(d: ResourceDraft): string {
  return d.resource.items_mode === "map"
    ? t("apiclients.detect.id_is_the_key")
    : t("apiclients.detect.id_is_the_value");
}

function summary(d: ResourceDraft): string {
  const ops = [d.resource.list?.operation, d.resource.get?.operation].filter(Boolean);
  return ops.join(" · ");
}

function fieldCount(d: ResourceDraft): number {
  return (d.resource.fields ?? []).length;
}

function apply(): void {
  emit(
    "apply",
    drafts.value.filter((_, i) => accepted.value[i]).map((d) => d.resource),
  );
}
</script>

<template>
  <Modal
    :open="open"
    :title="t('apiclients.detect.title')"
    width="760px"
    @close="emit('close')"
  >
    <p class="section-info">{{ t('apiclients.detect.info') }}</p>

    <p v-if="busy" class="muted small">{{ t('apiclients.detect.running') }}</p>
    <p v-else-if="drafts.length === 0" class="muted small">{{ emptyReason }}</p>

    <div v-else class="apiclients-detect-list">
      <div v-for="(d, i) in drafts" :key="d.resource.key" class="apiclients-detect-row">
        <SwitchField v-model="accepted[i]" />
        <div class="apiclients-detect-body">
          <div class="apiclients-detect-head">
            <code>{{ d.resource.key }}</code>
            <span v-if="d.resource.label" class="muted small">{{ d.resource.label }}</span>
            <span class="muted small">{{ summary(d) }}</span>
          </div>
          <div class="apiclients-detect-meta">
            <span v-if="d.resource.id_path" class="apiclients-chip">
              {{ t('apiclients.resource.id_path') }}: <code>{{ d.resource.id_path }}</code>
            </span>
            <span v-else class="apiclients-chip">{{ idOrigin(d) }}</span>
            <span v-if="d.resource.label_path" class="apiclients-chip">
              {{ t('apiclients.resource.label_path') }}: <code>{{ d.resource.label_path }}</code>
            </span>
            <span v-if="d.resource.items_mode === 'map'" class="apiclients-chip">
              {{ t('apiclients.resource.items_mode') }}:
              <code>{{ t('apiclients.resource.items_mode_map') }}</code>
            </span>
            <span v-if="d.resource.items_path" class="apiclients-chip">
              {{ t('apiclients.resource.items_path') }}: <code>{{ d.resource.items_path }}</code>
            </span>
            <span class="apiclients-chip">
              {{ t('apiclients.detect.fields', [String(fieldCount(d))]) }}
            </span>
          </div>
          <p v-if="(d.guessed ?? []).length > 0" class="apiclients-detect-guessed">
            <i class="fa-solid fa-triangle-exclamation" aria-hidden="true"></i>
            {{ t('apiclients.detect.guessed') }}
            <span v-for="attr in d.guessed" :key="attr" class="apiclients-chip">
              {{ guessLabel(attr) }}
            </span>
          </p>
        </div>
      </div>
    </div>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('close')">
        {{ t('common.cancel') }}
      </button>
      <button
        class="tool-btn primary"
        type="button"
        :disabled="busy || acceptedCount === 0"
        @click="apply"
      >
        {{ t('apiclients.detect.apply', [String(acceptedCount)]) }}
      </button>
    </template>
  </Modal>
</template>
