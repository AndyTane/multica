// @vitest-environment node
import { describe, expect, it } from "vitest";
import { hoursToSeconds } from "./duration-hours";

describe("duration hours input", () => {
  it.each([
    ["1.1", 3960], ["0.07", 252], ["0.5", 1800], ["0", 0], ["", 0],
    ["1.10", 3960], [".07", 252], ["7e-2", 252], ["1.1e1", 39600],
    ["2562047.7875", 9223372035],
  ])("converts %s hours to %s whole seconds", (text, seconds) => {
    expect(hoursToSeconds(String(text))).toBe(seconds);
  });
  it.each([
    "-1", "-0.07", "0.0001", "1.1000000000000001", "0.07000000000000001",
    "2562047.79", "1e1000", "1e-1000", "NaN", "Infinity", ".", "1e",
  ])("rejects %s without rounding it into a valid duration", (text) => {
    expect(hoursToSeconds(text)).toBeNaN();
  });
});
