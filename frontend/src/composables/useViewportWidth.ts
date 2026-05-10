import { onBeforeUnmount, onMounted, ref } from "vue";

// Reactive `window.innerWidth`. One listener per call site, cleaned up
// on unmount. Use the derived `narrow` flag to switch layouts at the
// shared collapse threshold.
const NARROW_BREAKPOINT = 740;

export function useViewportWidth() {
  const width = ref<number>(typeof window === "undefined" ? Infinity : window.innerWidth);
  const narrow = ref<boolean>(width.value < NARROW_BREAKPOINT);

  function update() {
    width.value = window.innerWidth;
    narrow.value = window.innerWidth < NARROW_BREAKPOINT;
  }

  onMounted(() => {
    update();
    window.addEventListener("resize", update);
  });
  onBeforeUnmount(() => {
    window.removeEventListener("resize", update);
  });

  return { width, narrow };
}
