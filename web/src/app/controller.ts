import {
  clearSavedCredential,
  loadSavedCredential,
  openConnection,
  testConnection,
  type ConnectionDependencies,
  type LoginCredentials,
} from "../connection/client";
import { createTerminalView, type FitAddonLike, type TerminalLike } from "../terminal/view";

export interface AppDependencies extends ConnectionDependencies {
  terminal: TerminalLike;
  fitAddon: FitAddonLike;
  ResizeObserver: typeof ResizeObserver;
}

type AppState = "connecting" | "connected" | "disconnected" | "exited";

const stateLabels: Record<AppState, string> = {
  connecting: "正在连接…",
  connected: "已连接",
  disconnected: "连接已断开",
  exited: "终端已退出",
};

export function createTerminalApp(deps: AppDependencies) {
  const elements = loginElements();
  const status = requireElement<HTMLElement>("status");
  const retry = requireElement<HTMLButtonElement>("retry");
  const view = createTerminalView({
    terminal: deps.terminal,
    fitAddon: deps.fitAddon,
    ResizeObserver: deps.ResizeObserver,
    element: requireElement<HTMLElement>("terminal"),
  });

  let sessionVersion = 0;
  let saved: { port: number; username: string } | undefined;

  function setState(state: AppState, label = stateLabels[state]) {
    status.textContent = label;
    status.dataset.state = state;
    retry.hidden = state !== "disconnected" && state !== "exited";
  }

  function readCredentials(): LoginCredentials {
    const port = Number(elements.port.value);
    const username = elements.username.value.trim();
    const useSavedCredential =
      saved?.port === port && saved.username === username && elements.password.value === "";
    if (
      !Number.isInteger(port) ||
      port < 1 ||
      port > 65535 ||
      !username ||
      (!useSavedCredential && !elements.password.value)
    ) {
      throw new Error("请检查端口、用户名和密码");
    }
    return {
      port,
      username,
      password: elements.password.value,
      useSavedCredential,
      remember: elements.remember.checked,
    };
  }

  function setBusy(busy: boolean) {
    for (const control of elements.controls) control.disabled = busy;
  }

  async function whileBusy(action: () => Promise<void>) {
    setBusy(true);
    try {
      await action();
    } finally {
      setBusy(false);
    }
  }

  function showDialog(message = "") {
    elements.message.textContent = message;
    if (!elements.dialog.open) elements.dialog.showModal();
    (saved ? elements.password : elements.username).focus();
  }

  function reflectSavedCredential(login?: LoginCredentials) {
    if (login?.remember) {
      saved = { port: login.port, username: login.username };
      elements.remember.checked = true;
      elements.password.placeholder = "已保存，无需重新输入";
      elements.clear.hidden = false;
      return;
    }
    saved = undefined;
    elements.remember.checked = false;
    elements.password.placeholder = "";
    elements.clear.hidden = true;
  }

  async function launch(login: LoginCredentials) {
    // 版本号使过期的 token/认证请求无法接管后来建立的会话。
    const currentVersion = ++sessionVersion;
    setState("connecting");
    try {
      const socket = await openConnection(deps, () => currentVersion === sessionVersion, login);
      if (!socket) return;

      elements.dialog.close();
      reflectSavedCredential(login);
      elements.password.value = "";
      view.attach(socket, (state, label) => {
        if (currentVersion === sessionVersion) setState(state, label);
      });
      setState("connected");
    } catch (error) {
      if (currentVersion !== sessionVersion) return;
      setState("disconnected", "无法连接终端");
      showDialog(errorMessage(error, "无法连接终端"));
    }
  }

  elements.form.addEventListener("submit", (event) => {
    event.preventDefault();
    try {
      const login = readCredentials();
      void whileBusy(() => launch(login));
    } catch (error) {
      elements.message.textContent = errorMessage(error, "请检查登录信息");
    }
  });

  elements.test.addEventListener("click", () => {
    try {
      const login = readCredentials();
      elements.message.textContent = "正在测试…";
      void whileBusy(async () => {
        try {
          elements.message.textContent = await testConnection(deps, login);
        } catch (error) {
          elements.message.textContent = errorMessage(error, "连接测试失败");
        }
      });
    } catch (error) {
      elements.message.textContent = errorMessage(error, "请检查登录信息");
    }
  });

  elements.clear.addEventListener("click", () => {
    void whileBusy(async () => {
      try {
        await clearSavedCredential(deps);
        reflectSavedCredential();
        elements.message.textContent = "已清除保存的登录信息";
      } catch {
        elements.message.textContent = "清除失败";
      }
    });
  });

  // 登录是建立终端的前置条件，Escape 不能绕过弹窗进入空白终端。
  elements.dialog.addEventListener("cancel", (event) => event.preventDefault());
  retry.addEventListener("click", () => showDialog());

  return {
    start: async () => {
      setState("disconnected", "等待连接");
      try {
        const value = await loadSavedCredential(deps);
        if (value.saved && value.username) {
          elements.port.value = String(value.port);
          elements.username.value = value.username;
          reflectSavedCredential({
            port: value.port,
            username: value.username,
            password: "",
            useSavedCredential: true,
            remember: true,
          });
        }
      } catch {
        elements.message.textContent = "无法读取已保存的登录信息";
      }
      showDialog(elements.message.textContent ?? "");
    },
    retry: () => showDialog(),
  };
}

function loginElements() {
  const port = requireElement<HTMLInputElement>("login-port");
  const username = requireElement<HTMLInputElement>("login-username");
  const password = requireElement<HTMLInputElement>("login-password");
  const remember = requireElement<HTMLInputElement>("login-remember");
  const test = requireElement<HTMLButtonElement>("test-connection");
  const connect = requireElement<HTMLButtonElement>("connect-terminal");
  const clear = requireElement<HTMLButtonElement>("clear-credentials");
  return {
    dialog: requireElement<HTMLDialogElement>("login-dialog"),
    form: requireElement<HTMLFormElement>("login-form"),
    port,
    username,
    password,
    remember,
    test,
    connect,
    clear,
    message: requireElement<HTMLElement>("login-message"),
    controls: [port, username, password, remember, test, connect, clear],
  };
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

function requireElement<T extends HTMLElement>(id: string): T {
  const element = document.getElementById(id);
  if (!element) throw new Error(`missing #${id}`);
  return element as T;
}
