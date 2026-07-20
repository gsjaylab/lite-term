import { beforeEach, describe, expect, it, vi } from "vitest";
import { createTerminalApp, type AppDependencies } from "./controller";

class FakeSocket {
  static instances: FakeSocket[] = [];
  binaryType = "";
  readyState = 0;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  sent: unknown[] = [];
  constructor(readonly url: string) { FakeSocket.instances.push(this); }
  send(data: unknown) {
    this.sent.push(data);
    if (typeof data === "string" && JSON.parse(data).type === "connect") queueMicrotask(() => this.onmessage?.(new MessageEvent("message", { data: '{"type":"authenticated"}' })));
  }
  open() { this.readyState = 1; this.onopen?.(new Event("open")); }
  close() { this.readyState = 3; this.onclose?.(); }
}

class FakeTerminal {
  cols = 80; rows = 24;
  loadAddon = vi.fn(); open = vi.fn(); focus = vi.fn(); write = vi.fn();
  onData() { return { dispose: vi.fn() }; }
}
class FakeResizeObserver { observe = vi.fn(); disconnect = vi.fn(); constructor(_callback: ResizeObserverCallback) {} }

describe("login terminal app", () => {
  let deps: AppDependencies;
  beforeEach(() => {
    document.body.innerHTML = `
      <div id="terminal"></div><span id="status"></span><button id="retry" hidden></button>
      <dialog id="login-dialog"><form id="login-form">
        <input id="login-port" value="22"><input id="login-username"><input id="login-password">
        <input id="login-remember" type="checkbox"><button id="test-connection" type="button"></button>
        <button id="connect-terminal" type="submit"></button><button id="clear-credentials" type="button" hidden></button>
        <p id="login-message"></p></form></dialog>`;
    const dialog = document.querySelector("dialog")!;
    dialog.showModal = vi.fn(() => dialog.setAttribute("open", ""));
    dialog.close = vi.fn(() => dialog.removeAttribute("open"));
    FakeSocket.instances = [];
    deps = {
      terminal: new FakeTerminal(), fitAddon: { fit: vi.fn() }, ResizeObserver: FakeResizeObserver as unknown as typeof ResizeObserver,
      WebSocket: FakeSocket as unknown as typeof WebSocket, location: { protocol: "https:", host: "nas.local", pathname: "/app/liteterm/" },
      fetch: vi.fn(async (input) => {
        const url = String(input);
        if (url.endsWith("/api/credentials")) return { ok: true, json: async () => ({ saved: false, port: 22 }) } as Response;
        return { ok: true, json: async () => ({ token: "once" }) } as Response;
      }) as unknown as typeof fetch,
    };
  });

  it("opens the login dialog first and does not create a terminal socket", async () => {
    const app = createTerminalApp(deps); await app.start();
    expect(document.querySelector("dialog")?.hasAttribute("open")).toBe(true);
    expect((document.querySelector("#login-port") as HTMLInputElement).value).toBe("22");
    expect(FakeSocket.instances).toHaveLength(0);
  });

  it("connects with form credentials and closes the dialog after authentication", async () => {
    const app = createTerminalApp(deps); await app.start();
    (document.querySelector("#login-username") as HTMLInputElement).value = "admin";
    (document.querySelector("#login-password") as HTMLInputElement).value = "secret";
    (document.querySelector("#login-remember") as HTMLInputElement).checked = true;
    document.querySelector("form")!.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
    await vi.waitFor(() => expect(FakeSocket.instances).toHaveLength(1));
    FakeSocket.instances[0].open();
    await vi.waitFor(() => expect(document.querySelector("#status")?.textContent).toBe("已连接"));
    expect(FakeSocket.instances[0].url).toContain("/app/liteterm/api/terminal?token=once");
    expect(FakeSocket.instances[0].sent[0]).toContain('"remember":true');
    expect(document.querySelector("dialog")?.hasAttribute("open")).toBe(false);
  });
});
