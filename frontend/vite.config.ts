import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import wails from "@wailsio/runtime/plugins/vite";

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue(), wails("./bindings")],
  server: {
    host: "127.0.0.1",
    strictPort: true,
  },
});
