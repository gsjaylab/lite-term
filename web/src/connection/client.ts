export interface ConnectionDependencies {
  fetch: typeof fetch;
  WebSocket: typeof WebSocket;
  location: Pick<Location, "protocol" | "host" | "pathname">;
}

export interface LoginCredentials {
  port: number;
  username: string;
  password: string;
  useSavedCredential: boolean;
  remember: boolean;
}

export async function loadSavedCredential(deps: ConnectionDependencies) {
  const response = await deps.fetch(`${applicationBase(deps.location.pathname)}/api/credentials`);
  if (!response.ok) throw new Error("credential request failed");
  const value: unknown = await response.json();
  if (!isSavedCredential(value)) throw new Error("invalid credential response");
  return value;
}

export async function clearSavedCredential(deps: ConnectionDependencies) {
  const response = await deps.fetch(`${applicationBase(deps.location.pathname)}/api/credentials`, { method: "DELETE" });
  if (!response.ok) throw new Error("credential deletion failed");
}

export async function testConnection(deps: ConnectionDependencies, credentials: LoginCredentials) {
  const response = await deps.fetch(`${applicationBase(deps.location.pathname)}/api/connection/test`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      port: credentials.port,
      username: credentials.username,
      password: credentials.password,
      useSavedCredential: credentials.useSavedCredential,
    }),
  });
  const payload = await response.json() as { ok?: boolean; message?: string };
  if (!response.ok || !payload.ok) throw new Error(payload.message || "连接测试失败");
  return payload.message || "连接测试成功";
}

export async function openConnection(
  deps: ConnectionDependencies,
  isCurrent: () => boolean,
  credentials: LoginCredentials,
): Promise<WebSocket | undefined> {
  const base = applicationBase(deps.location.pathname);
  const response = await deps.fetch(`${base}/api/token`, { method: "POST" });
  if (!response.ok) throw new Error("token request failed");
  const payload: unknown = await response.json();
  if (!hasToken(payload)) throw new Error("invalid token response");
  // token 请求可能晚于一次重试返回，旧请求不得再创建新 WebSocket。
  if (!isCurrent()) return undefined;

  const scheme = deps.location.protocol === "https:" ? "wss:" : "ws:";
  const url = `${scheme}//${deps.location.host}${base}/api/terminal?token=${encodeURIComponent(payload.token)}`;
  const socket = new deps.WebSocket(url);
  socket.binaryType = "arraybuffer";

  // WebSocket 成功升级不代表 SSH 已登录；只有服务端确认 authenticated 后，
  // 调用方才能把键盘输入交给终端视图。
  await authenticateSocket(socket, credentials);
  if (!isCurrent()) {
    socket.close();
    return undefined;
  }
  return socket;
}

function authenticateSocket(socket: WebSocket, credentials: LoginCredentials): Promise<void> {
  return new Promise((resolve, reject) => {
    let settled = false;
    const finish = (error?: Error) => {
      if (settled) return;
      settled = true;
      if (error) {
        if (socket.readyState < 2) socket.close();
        reject(error);
      } else {
        resolve();
      }
    };
    socket.onopen = () => {
      socket.send(JSON.stringify({ type: "connect", ...credentials, cols: 80, rows: 24 }));
    };
    socket.onmessage = (event) => {
      if (typeof event.data !== "string") {
        finish(new Error("服务器响应无效"));
        return;
      }
      try {
        const message = JSON.parse(event.data) as { type?: string; message?: string };
        if (message.type === "authenticated") finish();
        else if (message.type === "authentication_failed") finish(new Error(message.message || "连接失败"));
        else finish(new Error("服务器响应无效"));
      } catch {
        finish(new Error("服务器响应无效"));
      }
    };
    socket.onerror = () => finish(new Error("无法连接终端"));
    socket.onclose = () => finish(new Error("连接已断开"));
  });
}

export function applicationBase(pathname: string): string {
	// fnOS 统一网关把应用挂载在 /app/{appname}；开发服务器没有该前缀，
	// 此时返回空字符串即可复用同一套相对 API 路由。
  const match = pathname.match(/^\/app\/[A-Za-z0-9_-]+(?:\/|$)/);
  return match ? match[0].replace(/\/$/, "") : "";
}

function hasToken(value: unknown): value is { token: string } {
  return (
    typeof value === "object" &&
    value !== null &&
    typeof (value as { token?: unknown }).token === "string" &&
    (value as { token: string }).token.length > 0
  );
}

function isSavedCredential(value: unknown): value is { saved: boolean; port: number; username?: string } {
  if (typeof value !== "object" || value === null) return false;
  const candidate = value as { saved?: unknown; port?: unknown; username?: unknown };
  return (
    typeof candidate.saved === "boolean" &&
    Number.isInteger(candidate.port) &&
    Number(candidate.port) >= 1 &&
    Number(candidate.port) <= 65535 &&
    (!candidate.saved || typeof candidate.username === "string")
  );
}
