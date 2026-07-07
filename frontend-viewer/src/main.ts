import { createApp } from "vue";
import App from "./App.vue";
import { i18n, loadMessages } from "./i18n";
import { reportError } from "./state";
import "./styles.css";

async function boot(): Promise<void> {
  window.addEventListener("unhandledrejection", (ev) => reportError(ev.reason));
  try {
    await loadMessages();
  } catch (e) {
    // Backend not reachable yet: mount with the empty English fallback so the
    // window still renders rather than hanging on a white screen.
    reportError(e);
  }
  const app = createApp(App);
  app.config.errorHandler = (err) => reportError(err);
  app.use(i18n).mount("#app");
}

void boot();
