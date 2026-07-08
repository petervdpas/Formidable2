<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from "vue";

const props = defineProps<{
  name: string;
  title: string;
  description: string;
  wrong: boolean;
  busy?: boolean;
}>();
const emit = defineEmits<{ submit: [password: string]; cancel: [] }>();

const password = ref("");
const input = ref<HTMLInputElement | null>(null);

function focus(): void {
  void nextTick(() => input.value?.focus());
}
onMounted(focus);

// After a rejected attempt clear the field and refocus.
watch(
  () => props.wrong,
  (isWrong) => {
    if (isWrong) {
      password.value = "";
      focus();
    }
  },
);

function submit(): void {
  if (props.busy || !password.value) return;
  emit("submit", password.value);
}
</script>

<template>
  <div class="modal-backdrop" @click.self="emit('cancel')">
    <div class="modal unlock">
      <h2 class="modal-title">{{ $t("unlock.title") }}</h2>
      <div class="unlock-pack">
        <div class="unlock-pack-name">{{ title || name }}</div>
        <p v-if="description" class="unlock-pack-desc">{{ description }}</p>
      </div>
      <p class="field-help">{{ $t("unlock.hint") }}</p>
      <div class="field">
        <input
          ref="input"
          v-model="password"
          type="password"
          autocomplete="current-password"
          :placeholder="$t('unlock.password_placeholder')"
          @keydown.enter.prevent="submit"
        />
      </div>
      <p v-if="wrong" class="unlock-error">{{ $t("unlock.wrong") }}</p>
      <div class="modal-actions">
        <button class="btn ghost" @click="emit('cancel')">{{ $t("unlock.cancel") }}</button>
        <button class="btn primary" :disabled="!password || busy" @click="submit">
          {{ $t("unlock.submit") }}
        </button>
      </div>
    </div>
  </div>
</template>
