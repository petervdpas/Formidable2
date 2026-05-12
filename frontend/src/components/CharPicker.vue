<script setup lang="ts">
/*
 * CharPicker — Footer "P" button + popover grid of special characters.
 *
 * Insertion strategy: capture document.activeElement on mousedown of
 * the trigger button (before focus moves to the button), restore that
 * focus and use HTMLInputElement.setRangeText to splice the glyph at
 * the saved selection range. Native <input>/<textarea> only — CodeMirror
 * has its own dispatch and is opt-out for v1 (the button greys out and
 * shows a "click into a text field first" hint).
 *
 * The popover is closed on: outside-click, Escape, glyph insert, and
 * cleanup-on-unmount.
 */
import { computed, nextTick, onBeforeUnmount, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useToast } from "../composables/useToast";
import {
  CHAR_CATEGORIES,
  codepoint,
  type CharEntry,
} from "../utils/charpickerCatalog";

const { t } = useI18n();
const toast = useToast();

const open = ref(false);
const activeTab = ref<string>(CHAR_CATEGORIES[0]?.id ?? "arrows");
const search = ref("");
const recents = ref<CharEntry[]>([]);
const RECENTS_MAX = 16;

// Saved across mousedown→click so insertion targets the previously
// focused field, not the trigger button. selectionStart/End may be
// null on some elements (e.g. number inputs); insertion gates on that.
type SavedTarget = {
  el: HTMLInputElement | HTMLTextAreaElement;
  start: number;
  end: number;
};
let saved: SavedTarget | null = null;

function isInsertableField(el: Element | null): el is HTMLInputElement | HTMLTextAreaElement {
  if (!el) return false;
  if (el instanceof HTMLTextAreaElement) return true;
  if (el instanceof HTMLInputElement) {
    const t = (el.type || "text").toLowerCase();
    return t === "text" || t === "search" || t === "url" || t === "email" || t === "tel" || t === "password";
  }
  return false;
}

function captureTarget() {
  const el = document.activeElement;
  if (isInsertableField(el)) {
    saved = {
      el,
      start: el.selectionStart ?? el.value.length,
      end: el.selectionEnd ?? el.value.length,
    };
  } else {
    saved = null;
  }
}

const triggerRef = ref<HTMLButtonElement | null>(null);
const popoverRef = ref<HTMLDivElement | null>(null);
const searchRef = ref<HTMLInputElement | null>(null);

function onTriggerMouseDown() {
  // Runs before the click steals focus from the active field.
  if (!open.value) captureTarget();
}

async function toggle() {
  if (open.value) {
    close();
    return;
  }
  open.value = true;
  search.value = "";
  await nextTick();
  searchRef.value?.focus();
}

function close() {
  open.value = false;
  // Don't drop `saved` immediately — a stray click outside the field
  // would just leave the user disappointed; let the next open re-capture.
}

function onDocClick(e: MouseEvent) {
  if (!open.value) return;
  const t = e.target as Node | null;
  if (!t) return;
  if (popoverRef.value?.contains(t)) return;
  if (triggerRef.value?.contains(t)) return;
  close();
}
function onKeyDown(e: KeyboardEvent) {
  if (open.value && e.key === "Escape") {
    e.stopPropagation();
    close();
    triggerRef.value?.focus();
  }
}

document.addEventListener("mousedown", onDocClick, true);
document.addEventListener("keydown", onKeyDown);
onBeforeUnmount(() => {
  document.removeEventListener("mousedown", onDocClick, true);
  document.removeEventListener("keydown", onKeyDown);
});

function rememberRecent(entry: CharEntry) {
  const next = [entry, ...recents.value.filter((r) => r.char !== entry.char)];
  recents.value = next.slice(0, RECENTS_MAX);
}

function insert(entry: CharEntry) {
  if (!saved) {
    // No focused input/textarea was captured before the picker opened.
    // Toast the user; popover stays open so they can dismiss and retry
    // after clicking a field.
    toast.warn("charpicker.no_target");
    return;
  }
  const { el, start, end } = saved;
  el.focus();
  // setRangeText is the cleanest cross-browser path: it splices at the
  // saved selection, fires the native `input` event, and updates
  // selectionStart/End to live just after the inserted text.
  el.setRangeText(entry.char, start, end, "end");
  el.dispatchEvent(new Event("input", { bubbles: true }));
  el.dispatchEvent(new Event("change", { bubbles: true }));
  // Refresh `saved` so a follow-up insert lands AFTER the previous one,
  // not back at the original position.
  saved = {
    el,
    start: el.selectionStart ?? start + entry.char.length,
    end: el.selectionEnd ?? start + entry.char.length,
  };
  rememberRecent(entry);
}

const visibleEntries = computed<CharEntry[]>(() => {
  const q = search.value.trim().toLowerCase();
  if (q) {
    // Search across every category; recents excluded since they all
    // appear in their home category anyway.
    const all: CharEntry[] = [];
    for (const cat of CHAR_CATEGORIES) all.push(...cat.items);
    return all.filter((e) => e.name.includes(q) || e.char === q);
  }
  const cat = CHAR_CATEGORIES.find((c) => c.id === activeTab.value);
  return cat?.items ?? [];
});

const showRecents = computed(() => !search.value.trim() && recents.value.length > 0);
</script>

<template>
  <div class="char-picker">
    <button
      ref="triggerRef"
      type="button"
      class="char-picker-trigger"
      :class="{ 'is-open': open }"
      :title="t('charpicker.button.title')"
      :aria-label="t('charpicker.button.title')"
      :aria-expanded="open"
      @mousedown="onTriggerMouseDown"
      @click="toggle"
    >Ω</button>

    <div
      v-if="open"
      ref="popoverRef"
      class="char-picker-popover"
      role="dialog"
      :aria-label="t('charpicker.title')"
    >
      <div class="char-picker-header">
        <input
          ref="searchRef"
          v-model="search"
          type="search"
          class="char-picker-search"
          :placeholder="t('charpicker.search.placeholder')"
        />
      </div>

      <nav v-if="!search.trim()" class="char-picker-tabs" role="tablist">
        <button
          v-for="cat in CHAR_CATEGORIES"
          :key="cat.id"
          type="button"
          role="tab"
          :aria-selected="cat.id === activeTab"
          class="char-picker-tab"
          :class="{ active: cat.id === activeTab }"
          @click="activeTab = cat.id"
        >{{ t(cat.labelKey) }}</button>
      </nav>

      <div v-if="showRecents" class="char-picker-recents">
        <div class="char-picker-section-label">{{ t('charpicker.recents') }}</div>
        <div class="char-picker-grid">
          <button
            v-for="r in recents"
            :key="'r-' + r.char"
            type="button"
            class="char-picker-cell"
            :title="`${r.name} • ${codepoint(r.char)}`"
            @click="insert(r)"
          >{{ r.char }}</button>
        </div>
      </div>

      <div v-if="visibleEntries.length === 0" class="char-picker-empty">
        {{ t('charpicker.empty') }}
      </div>
      <div v-else class="char-picker-grid">
        <button
          v-for="e in visibleEntries"
          :key="e.char"
          type="button"
          class="char-picker-cell"
          :title="`${e.name} • ${codepoint(e.char)}`"
          @click="insert(e)"
        >{{ e.char }}</button>
      </div>
    </div>
  </div>
</template>
