import { expect, test, type WebSocketRoute } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  let socket: WebSocketRoute | undefined;

  await page.route("**/api/token", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ token: "fixture-token" }) }),
  );
	await page.route("**/api/credentials", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ saved: false, port: 22 }) }));
  await page.exposeFunction("__closeFixtureSocket", () => socket?.close());
  await page.addInitScript(() => {
    window.__closeFakeSocket = () => void window.__closeFixtureSocket();
  });
  await page.routeWebSocket("**/api/terminal?token=fixture-token", (webSocket) => {
    socket = webSocket;
    webSocket.onMessage((message) => {
      if (typeof message === "string") {
        const parsed: unknown = JSON.parse(message);
				if (typeof parsed === "object" && parsed !== null && (parsed as { type?: unknown }).type === "connect") {
					webSocket.send(JSON.stringify({ type: "authenticated", code: "", message: "" }));
					return;
				}
        if (typeof parsed === "object" && parsed !== null && (parsed as { type?: unknown }).type === "resize") {
          windowResize(page, parsed as { type: "resize"; cols: number; rows: number });
        }
        return;
      }
      webSocket.send(message);
    });
  });
});

async function login(page: import("@playwright/test").Page) {
	await expect(page.getByRole("dialog")).toBeVisible();
	await page.locator("#login-username").fill("admin");
	await page.locator("#login-password").fill("secret");
	await page.getByRole("button", { name: "连接", exact: true }).click();
	await expect(page.locator("#status")).toHaveText("已连接");
}

function windowResize(page: import("@playwright/test").Page, resize: { type: "resize"; cols: number; rows: number }) {
  void page.evaluate((value) => {
    window.__lastResize = value;
  }, resize);
}

test("opens one terminal, fits it, and exposes retry after disconnect", async ({ page }) => {
  await page.goto("/");
	await login(page);
  await expect(page.getByText("单会话终端")).toHaveCount(0);
  await expect(page.locator("#status")).toHaveText("已连接");
  await expect(page.locator(".terminal-bar #status")).toBeVisible();
  await page.keyboard.type("printf 你好");
  await expect(page.locator(".xterm-rows")).toContainText("你好");
  await page.setViewportSize({ width: 900, height: 600 });
  await expect.poll(() => page.evaluate(() => window.__lastResize)).toMatchObject({ type: "resize" });
  await page.evaluate(() => window.__closeFakeSocket());
  await expect(page.locator("#status")).toHaveText("连接已断开");
  await expect(page.getByRole("button", { name: "重新打开终端" })).toBeVisible();
});

test("keeps the document height stable in a short fnOS window", async ({ page }) => {
  await page.setViewportSize({ width: 1000, height: 480 });
  await page.goto("/");
	await login(page);

  const heights: number[] = [];
  for (let sample = 0; sample < 5; sample += 1) {
    await page.waitForTimeout(200);
    heights.push(await page.evaluate(() => document.documentElement.scrollHeight));
  }

  expect(Math.max(...heights) - Math.min(...heights)).toBeLessThanOrEqual(1);

  const terminalBounds = await page.locator("#terminal").boundingBox();
  const lastRowBounds = await page.locator(".xterm-rows > div").last().boundingBox();
  expect(terminalBounds).not.toBeNull();
  expect(lastRowBounds).not.toBeNull();
  expect(lastRowBounds!.y + lastRowBounds!.height).toBeLessThanOrEqual(terminalBounds!.y + terminalBounds!.height - 12);
});

declare global {
  interface Window {
    __lastResize?: { type: "resize"; cols: number; rows: number };
    __closeFakeSocket: () => void;
    __closeFixtureSocket: () => Promise<void>;
  }
}
