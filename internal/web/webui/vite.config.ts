import { defineConfig } from "vite";
import solid from "vite-plugin-solid";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [solid(), tailwindcss()],
  // Relative base lets the embedded index.html load assets regardless of the
  // path the SPA is mounted under.
  base: "./",
  build: {
    target: "es2020",
    cssCodeSplit: false,
    outDir: "dist",
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://127.0.0.1:9000",
      "/ws": { target: "ws://127.0.0.1:9000", ws: true },
      "/metrics": "http://127.0.0.1:9000",
    },
  },
});
