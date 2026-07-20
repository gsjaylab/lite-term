import { decodeServerMessage, encodeResize } from "../protocol/messages";

type Disposable = { dispose(): void };

export interface TerminalLike {
  cols: number;
  rows: number;
  loadAddon(addon: unknown): void;
  open(element: HTMLElement): void;
  focus(): void;
  write(data: string): void;
  onData(handler: (data: string) => void): Disposable;
}

export interface FitAddonLike {
  fit(): void;
}

interface TerminalViewDependencies {
  terminal: TerminalLike;
  fitAddon: FitAddonLike;
  ResizeObserver: typeof ResizeObserver;
  element: HTMLElement;
}

type TerminalFinish = (state: "disconnected" | "exited", label?: string) => void;

export function createTerminalView(deps: TerminalViewDependencies) {
  const encoder = new TextEncoder();
  deps.terminal.loadAddon(deps.fitAddon);
  deps.terminal.open(deps.element);

  function attach(socket: WebSocket, finishApp: TerminalFinish): Disposable {
    const decoder = new TextDecoder();
    let finished = false;
    let resizeTimer: ReturnType<typeof setTimeout> | undefined;
    let inputSubscription: Disposable | undefined;

    const observer = new deps.ResizeObserver(() => {
      if (resizeTimer !== undefined) clearTimeout(resizeTimer);
      resizeTimer = setTimeout(() => {
        // fit 必须先更新 cols/rows，再发送尺寸；短窗口的瞬时零尺寸留给下一次 observer 重试。
        deps.fitAddon.fit();
        sendResize(socket, deps.terminal);
      }, 50);
    });

    const cleanUp = () => {
      inputSubscription?.dispose();
      inputSubscription = undefined;
      observer.disconnect();
      if (resizeTimer !== undefined) clearTimeout(resizeTimer);
    };

    const flushDecoder = () => {
      // 流式解码器可能缓存半个多字节字符，结束时必须 flush，不能静默丢失尾部数据。
      const tail = decoder.decode();
      if (tail) deps.terminal.write(tail);
    };

    const finish = (state: "disconnected" | "exited", label?: string) => {
      if (finished) return;
      finished = true;
      flushDecoder();
      cleanUp();
      finishApp(state, label);
    };

    const start = () => {
      inputSubscription = deps.terminal.onData((data) => {
        if (!finished && socket.readyState === 1) socket.send(encoder.encode(data));
      });
      observer.observe(deps.element);
      deps.fitAddon.fit();
      sendResize(socket, deps.terminal);
      deps.terminal.focus();
    };
		socket.onopen = start;
		if (socket.readyState === 1) start();

    socket.onmessage = (event) => {
      if (finished) return;
      if (typeof event.data === "string") {
        try {
          const message = decodeServerMessage(event.data);
          finish("exited", `终端已退出（代码 ${message.code}）`);
          socket.close();
        } catch {
          finish("disconnected", "收到无效的服务器消息");
          socket.close();
        }
        return;
      }
      if (event.data instanceof ArrayBuffer) {
        deps.terminal.write(decoder.decode(new Uint8Array(event.data), { stream: true }));
      } else if (ArrayBuffer.isView(event.data)) {
        deps.terminal.write(
          decoder.decode(new Uint8Array(event.data.buffer, event.data.byteOffset, event.data.byteLength), { stream: true }),
        );
      }
    };

    socket.onclose = () => finish("disconnected");
    return { dispose: () => finish("disconnected") };
  }

  return { attach };
}

function sendResize(socket: WebSocket, terminal: TerminalLike) {
  if (socket.readyState !== 1) return;
  try {
    socket.send(encodeResize(terminal.cols, terminal.rows));
  } catch {
    // 瞬时零尺寸会由下一次 ResizeObserver 事件重试。
  }
}
