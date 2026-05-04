import { defineConfig } from "vite";
import wails from "@wailsio/runtime/plugins/vite";

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [wails("./bindings")],
  server: {
    host: "127.0.0.1",
    strictPort: true,
  },
});
