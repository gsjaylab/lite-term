import { defineConfig } from "vitest/config";

export default defineConfig({
  base: "/app/liteterm/",
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.ts"],
    restoreMocks: true,
    setupFiles: ["./src/test-setup.ts"],
  },
});
