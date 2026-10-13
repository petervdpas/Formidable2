import { ref } from "vue";

// A question for the app's own ConfirmDialog, with what to do on yes. A
// component keeps one `ask`, binds it to a <ConfirmDialog>, and calls
// request() where it would otherwise reach for the browser's confirm(), which
// shows an unstyled box outside the app's look.
export interface ConfirmAsk {
  title: string;
  message: string;
  confirmLabel: string;
  run: () => void | Promise<void>;
}

export function useConfirmAsk() {
  const ask = ref<ConfirmAsk | null>(null);

  function request(next: ConfirmAsk): void {
    ask.value = next;
  }

  async function confirm(): Promise<void> {
    const current = ask.value;
    ask.value = null;
    if (current) await current.run();
  }

  function cancel(): void {
    ask.value = null;
  }

  return { ask, request, confirm, cancel };
}
