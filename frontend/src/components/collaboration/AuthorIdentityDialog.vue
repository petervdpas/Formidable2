<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "../Modal.vue";
import { FormSection, FormRow, TextField } from "../fields";
import {
  isValidAuthorName,
  isValidAuthorEmail,
} from "../../composables/useAuthorValidation";

// Author-identity capture modal. Pops from Sync.vue when the user
// tries to commit while the active profile still has the seeded
// "unknown" / "@example.com" defaults (or anything that fails the
// shared validators). On Save we emit the new identity; the parent
// writes it to config and proceeds with the commit.

const props = defineProps<{
  open: boolean;
  initialName: string;
  initialEmail: string;
}>();

const emit = defineEmits<{
  (e: "save", name: string, email: string): void;
  (e: "cancel"): void;
}>();

const { t } = useI18n();

const name = ref(props.initialName);
const email = ref(props.initialEmail);

watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) {
      name.value = props.initialName;
      email.value = props.initialEmail;
    }
  },
);

const nameInvalid = computed(() => !isValidAuthorName(name.value));
const emailInvalid = computed(() => !isValidAuthorEmail(email.value));
const canSave = computed(() => !nameInvalid.value && !emailInvalid.value);

function save() {
  if (!canSave.value) return;
  emit("save", name.value.trim(), email.value.trim());
}
</script>

<template>
  <Modal
    :open="open"
    :title="t('workspace.collaboration.author_dialog.title')"
    @close="emit('cancel')"
  >
    <p class="section-info" v-html="t('workspace.collaboration.author_dialog.body')"></p>

    <FormSection>
      <FormRow
        :label="t('config.author_name')"
        :error="nameInvalid && name !== '' ? t('workspace.collaboration.author_dialog.name_invalid') : undefined"
      >
        <TextField
          v-model="name"
          :invalid="nameInvalid && name !== ''"
          :placeholder="t('workspace.collaboration.author_dialog.name_placeholder')"
        />
      </FormRow>
      <FormRow
        :label="t('config.author_email')"
        :error="emailInvalid && email !== '' ? t('workspace.collaboration.author_dialog.email_invalid') : undefined"
      >
        <TextField
          v-model="email"
          type="email"
          :invalid="emailInvalid && email !== ''"
          :placeholder="t('workspace.collaboration.author_dialog.email_placeholder')"
          @keydown.enter="save"
        />
      </FormRow>
    </FormSection>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('cancel')">
        {{ t('common.cancel') }}
      </button>
      <button
        class="tool-btn primary"
        type="button"
        :disabled="!canSave"
        @click="save"
      >
        {{ t('workspace.collaboration.author_dialog.save') }}
      </button>
    </template>
  </Modal>
</template>
