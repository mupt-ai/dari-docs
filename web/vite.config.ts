import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, ".", "");
  const apiProxyTarget = env.VITE_API_PROXY_TARGET || "http://localhost:8080";
  const allowedHostsRaw =
    process.env.VITE_ALLOWED_HOSTS ?? env.VITE_ALLOWED_HOSTS ?? "";
  const allowedHosts = allowedHostsRaw
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);

  return {
    plugins: [react()],
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
      },
    },
    server: {
      port: 5173,
      ...(allowedHosts.length > 0 ? { allowedHosts } : {}),
      proxy: {
        "/v1": {
          target: apiProxyTarget,
          changeOrigin: true,
        },
        "/_dev": {
          target: apiProxyTarget,
          changeOrigin: true,
        },
      },
    },
  };
});
