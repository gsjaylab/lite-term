type ServerMessage = { type: "exit"; code: number };

export function encodeResize(cols: number, rows: number): string {
  if (
    !Number.isInteger(cols) ||
    !Number.isInteger(rows) ||
    cols < 2 ||
    cols > 300 ||
    rows < 1 ||
    rows > 150
  ) {
    throw new Error("invalid terminal size");
  }

  return JSON.stringify({ type: "resize", cols, rows });
}

export function decodeServerMessage(raw: string): ServerMessage {
  const value: unknown = JSON.parse(raw);
  if (
    typeof value !== "object" ||
    value === null ||
    (value as ServerMessage).type !== "exit" ||
    !Number.isInteger((value as ServerMessage).code)
  ) {
    throw new Error("invalid server message");
  }

  return value as ServerMessage;
}
