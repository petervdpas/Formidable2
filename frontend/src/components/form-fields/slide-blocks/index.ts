// Registry mapping each reveal element kind to its component. Each component
// renders both surfaces (canvas preview + inspector editor) via a `surface`
// prop, so adding a new element type means adding one file and one entry here.
import type { Component } from "vue";
import SlideText from "./SlideText.vue";
import SlideImage from "./SlideImage.vue";
import SlideVideo from "./SlideVideo.vue";
import SlideEmbed from "./SlideEmbed.vue";
import SlideCode from "./SlideCode.vue";
import SlideMath from "./SlideMath.vue";
import SlideTable from "./SlideTable.vue";
import SlideList from "./SlideList.vue";
import SlideQuote from "./SlideQuote.vue";
import SlideMermaid from "./SlideMermaid.vue";

export const SLIDE_BLOCK_COMPONENTS: Record<string, Component> = {
  text: SlideText,
  image: SlideImage,
  video: SlideVideo,
  embed: SlideEmbed,
  code: SlideCode,
  math: SlideMath,
  table: SlideTable,
  list: SlideList,
  quote: SlideQuote,
  mermaid: SlideMermaid,
};

// slideBlockComponent resolves a kind to its component, falling back to text.
export function slideBlockComponent(kind: string): Component {
  return SLIDE_BLOCK_COMPONENTS[kind] ?? SlideText;
}

export { default as SlideSettings } from "./SlideSettings.vue";
export { default as SlideElementTransition } from "./SlideElementTransition.vue";
export { default as SlideElementOrder } from "./SlideElementOrder.vue";
