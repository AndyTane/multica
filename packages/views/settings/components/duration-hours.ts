import { MAX_TIMEOUT_SECONDS } from "@multica/core/agents/execution-timeouts";

/** Convert the decimal input exactly; fractional seconds must not be rounded. */
export function hoursToSeconds(text: string): number {
  if (text === "") return 0;
  const match = /^([+-]?)(\d*)(?:\.(\d*))?(?:e([+-]?\d+))?$/i.exec(text);
  if (!match || !(match[2] || match[3])) return NaN;
  const fraction = match[3] ?? "";
  const coefficient = BigInt(match[2] + fraction) * 3600n;
  if (coefficient === 0n) return 0;
  const scale = fraction.length - Number(match[4] ?? "0");
  if (!Number.isSafeInteger(scale)) return NaN;
  const digits = coefficient.toString();
  let seconds: number;
  if (scale > 0) {
    if (scale >= digits.length || /[1-9]/.test(digits.slice(-scale))) return NaN;
    seconds = Number(digits.slice(0, -scale));
  } else {
    seconds = Number(coefficient) * 10 ** -scale;
  }
  if (match[1] === "-") seconds = -seconds;
  return Number.isInteger(seconds) && seconds >= 0 && seconds <= MAX_TIMEOUT_SECONDS ? seconds : NaN;
}
