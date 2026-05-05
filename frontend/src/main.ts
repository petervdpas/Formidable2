import "./styles/index.css";
import { createApp } from "vue";
import App from "./App.vue";
import { i18n } from "./i18n";
import { useI18nLoader } from "./composables/useI18nLoader";

// Kick off bundle load before mount so first paint already has the
// active locale. The composable resolves itself on the second boot.
useI18nLoader();

createApp(App).use(i18n).mount("#app");
