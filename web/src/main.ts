import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { createTerminalApp } from "./app/controller";
import "./styles/app.css";

if (document.getElementById("terminal")) {
  const terminal = new Terminal({
    convertEol: true,
    cursorBlink: true,
    fontFamily: '"IBM Plex Mono", "SFMono-Regular", Consolas, monospace',
    fontSize: 12,
    lineHeight: 1.15,
    theme: {
      background: "#101722",
      foreground: "#d9e3ee",
      cursor: "#7dd3fc",
      selectionBackground: "#27435c",
    },
  });
  const app = createTerminalApp({
    terminal,
    fitAddon: new FitAddon(),
    fetch: window.fetch.bind(window),
    WebSocket: window.WebSocket,
    ResizeObserver: window.ResizeObserver,
    location: window.location,
  });
  void app.start();
}
