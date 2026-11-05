import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  base: "/",
  server: {
    host: "127.0.0.1",
    port: 5174,
    strictPort: true,
    proxy: {
      "/api/v1/admin": {
        target: process.env.ADMIN_API_PROXY_TARGET || "http://127.0.0.1:8082",
        changeOrigin: false,
      },
    },
  },
  build: {
    target: ["chrome120", "firefox120", "safari17"],
    sourcemap: false,
  },
});
