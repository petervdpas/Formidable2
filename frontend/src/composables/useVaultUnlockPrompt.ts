import { ref } from "vue";

// A single app-wide prompt for the vault's master password. Anything that
// finds the vault locked in the middle of what the user asked for calls
// askVaultUnlock() and waits: it resolves true once the vault is open, false
// when the user cancels. The dialog itself (VaultUnlockDialog, mounted once in
// App.vue) does the unlocking; callers never see the password.
const open = ref<boolean>(false);
let resolver: ((unlocked: boolean) => void) | null = null;

export function askVaultUnlock(): Promise<boolean> {
  // A second request while the dialog is up joins the first one's answer.
  if (open.value && resolver) {
    const first = resolver;
    return new Promise<boolean>((resolve) => {
      resolver = (v) => {
        first(v);
        resolve(v);
      };
    });
  }
  open.value = true;
  return new Promise<boolean>((resolve) => {
    resolver = resolve;
  });
}

export function useVaultUnlockPrompt() {
  function settle(unlocked: boolean): void {
    open.value = false;
    const r = resolver;
    resolver = null;
    r?.(unlocked);
  }
  return { open, settle };
}
