import { describe, expect, it } from "vitest";
import config from "../vite.config";

describe("production asset base", () => {
  it("keeps assets under the fnOS gateway prefix", () => {
    expect(config.base).toBe("/app/liteterm/");
  });
});
