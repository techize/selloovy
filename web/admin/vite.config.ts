import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

export default defineConfig({
  plugins: [vue()],
  base: "/admin/",
  envPrefix: "SELLOOVY_PUBLIC_",
  build: { sourcemap: false },
});
