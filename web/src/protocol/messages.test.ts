import { describe, expect, it } from "vitest";
import { decodeServerMessage, encodeResize } from "./messages";

describe("terminal protocol", () => {
  it("encodes a bounded resize", () => {
    expect(encodeResize(80, 24)).toBe('{"type":"resize","cols":80,"rows":24}');
    expect(() => encodeResize(301, 24)).toThrow("invalid terminal size");
  });

  it("decodes exit messages", () => {
    expect(decodeServerMessage('{"type":"exit","code":7}')).toEqual({ type: "exit", code: 7 });
  });
});
