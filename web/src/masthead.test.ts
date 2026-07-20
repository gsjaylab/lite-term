import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const html = readFileSync(resolve(process.cwd(), "index.html"), "utf8");

describe("page masthead", () => {
  it("keeps status inside the terminal toolbar without duplicate labels", () => {
    const document = new DOMParser().parseFromString(html, "text/html");

    expect(document.querySelector(".brand")).toBeNull();
    expect(document.querySelector(".terminal-bar .connection #status")).not.toBeNull();
    expect(document.querySelector(".terminal-bar p")).toBeNull();
    expect(html).not.toContain("单会话终端");
		expect(document.querySelector(".terminal-bar h1")).toBeNull();
    expect(document.querySelector("main")?.getAttribute("aria-label")).toBe("轻终端");
    expect(document.querySelector("main")?.hasAttribute("aria-labelledby")).toBe(false);
  });
});
