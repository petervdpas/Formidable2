<script setup lang="ts">
// Reveal "shape" element: a vector primitive (rectangle/ellipse/triangle/line/
// arrow), OR an imported SVG that takes over the block. The block's box gives
// position and size; content holds the variant + paint, or `svgFile` (a filename
// under the template's images/ folder). An imported SVG is sanitized on import,
// stored as an asset like any image, and rendered as an <img> (which the browser
// sandboxes). The canvas shows the backend-rendered output live, same as every
// other block.
import { computed, inject, ref, type ComputedRef } from "vue";
import { useI18n } from "vue-i18n";
import { Service as RenderSvc } from "../../../../bindings/github.com/petervdpas/formidable2/internal/modules/render";
import { Service as StorageSvc } from "../../../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import SlideRenderedPreview from "./SlideRenderedPreview.vue";
import { SHAPE_DEFAULT, type SlideBlock } from "../../../types/slide-blocks";

const props = defineProps<{ block: SlideBlock; surface: "canvas" | "inspector"; html?: string }>();
const emit = defineEmits<{ (e: "patch", p: Partial<SlideBlock>): void }>();

const { t } = useI18n();

// The template's YAML filename, provided by StorageWorkspace (as for the image field).
const templateFilename = inject<ComputedRef<string>>("templateFilename", computed(() => ""));

// Explicit key maps (never interpolate the i18n key) so extraction/grep see them.
const VARIANTS = [
  { value: "rectangle", labelKey: "workspace.storage.slide.shape.rectangle" },
  { value: "ellipse", labelKey: "workspace.storage.slide.shape.ellipse" },
  { value: "triangle", labelKey: "workspace.storage.slide.shape.triangle" },
  { value: "line", labelKey: "workspace.storage.slide.shape.line" },
  { value: "arrow", labelKey: "workspace.storage.slide.shape.arrow" },
] as const;

const DIRECTIONS = [
  { value: "horizontal", labelKey: "workspace.storage.slide.shape.horizontal" },
  { value: "vertical", labelKey: "workspace.storage.slide.shape.vertical" },
  { value: "diagonal-down", labelKey: "workspace.storage.slide.shape.diagonal_down" },
  { value: "diagonal-up", labelKey: "workspace.storage.slide.shape.diagonal_up" },
] as const;

const NO_FILL = "none";

interface ShapeContent {
  shape: string;
  fill: string;
  stroke: string;
  strokeWidth: number;
  direction: string;
  svgFile: string;
  tint: string;
}

const cur = computed<ShapeContent>(() => {
  const c = (props.block.content ?? {}) as Record<string, unknown>;
  const width = Number(c.strokeWidth);
  return {
    shape: typeof c.shape === "string" ? c.shape : SHAPE_DEFAULT.shape,
    fill: typeof c.fill === "string" ? c.fill : SHAPE_DEFAULT.fill,
    stroke: typeof c.stroke === "string" ? c.stroke : SHAPE_DEFAULT.stroke,
    strokeWidth: Number.isFinite(width) ? width : SHAPE_DEFAULT.strokeWidth,
    direction: typeof c.direction === "string" ? c.direction : "horizontal",
    svgFile: typeof c.svgFile === "string" ? c.svgFile : "",
    tint: typeof c.tint === "string" ? c.tint : "",
  };
});

// An imported SVG takes over the block; the primitive controls hide while it is set.
const hasSvg = computed(() => cur.value.svgFile !== "");
// Rectangle/ellipse/triangle enclose an area, so fill applies. Line/arrow are
// stroke-only, and get a direction instead.
const isFillable = computed(() => ["rectangle", "ellipse", "triangle"].includes(cur.value.shape));
const isLinear = computed(() => cur.value.shape === "line" || cur.value.shape === "arrow");
// "none" paint (transparent). The colour picker can't represent it, so the
// fa-ban button carries that state, mirroring the text block's Color/Background.
const noFill = computed(() => cur.value.fill === NO_FILL);
const noStroke = computed(() => cur.value.stroke === NO_FILL);

function update(p: Partial<ShapeContent>) {
  emit("patch", { content: { ...cur.value, ...p } });
}

const fileInput = ref<HTMLInputElement | null>(null);
const importError = ref(false);

function pickFile() {
  importError.value = false;
  fileInput.value?.click();
}

// Wails marshals Go []byte as a base64 string; chunk to dodge the call-stack
// ceiling on large blobs (mirrors FormFieldImage).
function bytesToBase64(bytes: Uint8Array): string {
  let binary = "";
  const CHUNK = 0x8000;
  for (let i = 0; i < bytes.length; i += CHUNK) {
    binary += String.fromCharCode.apply(null, Array.from(bytes.subarray(i, i + CHUNK)));
  }
  return btoa(binary);
}

// Sanitize the picked file, store the clean SVG as an image asset, and reference
// it by filename. One filename per block, so re-importing overwrites in place.
async function onFile(e: Event) {
  const input = e.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = ""; // let the same file be re-picked
  if (!file) return;
  importError.value = false;
  if (!templateFilename.value) {
    importError.value = true;
    return;
  }
  const clean = await RenderSvc.SanitizeSVG(await file.text());
  if (!clean) {
    importError.value = true;
    return;
  }
  const name = `shape-${props.block.id}.svg`;
  const b64 = bytesToBase64(new TextEncoder().encode(clean));
  const result = await StorageSvc.SaveImageFile(templateFilename.value, name, b64);
  if (!result?.success) {
    importError.value = true;
    return;
  }
  update({ svgFile: name });
}

async function removeSvg() {
  const name = cur.value.svgFile;
  if (name && templateFilename.value) {
    try {
      await StorageSvc.DeleteImageFile(templateFilename.value, name);
    } catch {
      // Best-effort: clear the reference even if the file delete fails.
    }
  }
  update({ svgFile: "", tint: "" });
}
</script>

<template>
  <SlideRenderedPreview v-if="surface === 'canvas'" :block="block" :html="html" />
  <template v-else>
    <template v-if="hasSvg">
      <div class="muted small">{{ t('workspace.storage.slide.shape.svg_imported') }}</div>
      <div class="slide-style-color">
        <span>{{ t('workspace.storage.slide.shape.tint') }}</span>
        <input
          type="color" :value="cur.tint || '#000000'"
          @input="update({ tint: ($event.target as HTMLInputElement).value })"
        />
        <button
          type="button" class="slide-style-clear" :class="{ active: !cur.tint }"
          :title="t('workspace.storage.slide.shape.tint_original')" @click="update({ tint: '' })"
        ><i class="fa-solid fa-ban" aria-hidden="true"></i></button>
      </div>
      <button type="button" class="tool-btn danger" @click="removeSvg">
        <i class="fa-solid fa-trash-can" aria-hidden="true"></i> {{ t('workspace.storage.slide.shape.remove_svg') }}
      </button>
    </template>
    <template v-else>
      <label class="slide-inspector-row">
        {{ t('workspace.storage.slide.shape.variant') }}
        <select :value="cur.shape" @change="update({ shape: ($event.target as HTMLSelectElement).value })">
          <option v-for="v in VARIANTS" :key="v.value" :value="v.value">{{ t(v.labelKey) }}</option>
        </select>
      </label>
      <label v-if="isLinear" class="slide-inspector-row">
        {{ t('workspace.storage.slide.shape.direction') }}
        <select :value="cur.direction" @change="update({ direction: ($event.target as HTMLSelectElement).value })">
          <option v-for="d in DIRECTIONS" :key="d.value" :value="d.value">{{ t(d.labelKey) }}</option>
        </select>
      </label>
      <div v-if="isFillable" class="slide-style-color">
        <span>{{ t('workspace.storage.slide.shape.fill') }}</span>
        <input
          type="color" :value="noFill ? SHAPE_DEFAULT.fill : cur.fill"
          @input="update({ fill: ($event.target as HTMLInputElement).value })"
        />
        <button
          type="button" class="slide-style-clear" :class="{ active: noFill }"
          :title="t('workspace.storage.slide.shape.no_fill')" @click="update({ fill: NO_FILL })"
        ><i class="fa-solid fa-ban" aria-hidden="true"></i></button>
      </div>
      <div class="slide-style-color">
        <span>{{ t('workspace.storage.slide.shape.stroke') }}</span>
        <input
          type="color" :value="noStroke ? SHAPE_DEFAULT.stroke : cur.stroke"
          @input="update({ stroke: ($event.target as HTMLInputElement).value })"
        />
        <button
          type="button" class="slide-style-clear" :class="{ active: noStroke }"
          :title="t('workspace.storage.slide.no_color')" @click="update({ stroke: NO_FILL })"
        ><i class="fa-solid fa-ban" aria-hidden="true"></i></button>
      </div>
      <label class="slide-inspector-row">
        {{ t('workspace.storage.slide.shape.stroke_width') }}
        <input
          type="number" min="0" max="100" step="1" :value="cur.strokeWidth"
          @input="update({ strokeWidth: Number(($event.target as HTMLInputElement).value) })"
        />
      </label>
      <button type="button" class="tool-btn" @click="pickFile">
        <i class="fa-solid fa-file-import" aria-hidden="true"></i> {{ t('workspace.storage.slide.shape.import_svg') }}
      </button>
      <div v-if="importError" class="error small">{{ t('workspace.storage.slide.shape.svg_invalid') }}</div>
    </template>
    <input ref="fileInput" type="file" accept=".svg,image/svg+xml" hidden @change="onFile" />
  </template>
</template>
