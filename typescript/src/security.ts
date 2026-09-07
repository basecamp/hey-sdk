import { parse, parseNumberAndBigInt, stringify } from "lossless-json";
import { HeyError } from "./errors.js";
export function secureURL(raw: string | URL): URL {
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    throw new HeyError("usage", "Invalid endpoint URL");
  }
  const host = url.hostname;
  const local =
    host === "localhost" ||
    host.endsWith(".localhost") ||
    host === "127.0.0.1" ||
    host === "[::1]";
  if (
    url.username ||
    url.password ||
    url.hash ||
    (url.protocol !== "https:" && !(url.protocol === "http:" && local))
  )
    throw new HeyError(
      "usage",
      "Endpoint must use HTTPS (HTTP is allowed only on localhost), without credentials or fragment",
    );
  return url;
}
export function sameOriginURL(raw: string, base: URL): URL {
  const target = new URL(raw, base);
  if (target.origin !== base.origin)
    throw new HeyError("usage", "URL points to a different origin");
  return secureURL(target);
}
export function parseJSON(text: string): unknown {
  return parse(text, undefined, (value) => {
    const number = parseNumberAndBigInt(value);
    if (
      typeof number === "number" &&
      (!Number.isFinite(number) ||
        (Number.isInteger(number) && !Number.isSafeInteger(number)))
    ) {
      throw new Error(
        "Unsafe numeric encoding; large integers must be integer JSON literals",
      );
    }
    return typeof number === "bigint" && Number.isSafeInteger(Number(number))
      ? Number(number)
      : number;
  });
}
export function assertSafeNumbers(value: unknown): void {
  if (
    typeof value === "number" &&
    (!Number.isFinite(value) ||
      (Number.isInteger(value) && !Number.isSafeInteger(value)))
  )
    throw new HeyError(
      "usage",
      "Unsafe number; use bigint for integers outside the safe range",
    );
  if (Array.isArray(value)) value.forEach(assertSafeNumbers);
  else if (value && typeof value === "object")
    Object.values(value).forEach(assertSafeNumbers);
}
export function stringifyJSON(value: unknown): string {
  assertSafeNumbers(value);
  return stringify(value) ?? "";
}
export function networkFailure(cause: unknown, signal?: AbortSignal): never {
  if (signal?.aborted) throw signal.reason;
  if (cause instanceof HeyError) throw cause;
  throw new HeyError("network", "Network request failed", 0, true, "", "", {
    cause,
  });
}
export async function readBounded(
  response: Response,
  max: number,
  signal?: AbortSignal,
): Promise<string> {
  if (!response.body) return "";
  const reader = response.body.getReader();
  let size = 0;
  const chunks: Uint8Array[] = [];
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.byteLength;
      if (size > max) {
        const error = new HeyError(
          "response_too_large",
          `Response exceeds ${max} bytes`,
          response.status,
          false,
          response.headers.get("X-Request-Id") ?? "",
        );
        try {
          await reader.cancel();
        } catch {
          // The size violation remains authoritative if stream cancellation also fails.
        }
        throw error;
      }
      chunks.push(value);
    }
    return Buffer.concat(chunks).toString("utf8");
  } catch (cause) {
    networkFailure(cause, signal);
  } finally {
    reader.releaseLock();
  }
}
function splitOutsideQuotes(value: string, separator: string): string[] {
  const parts: string[] = [];
  let start = 0;
  let quoted = false;
  let escaped = false;
  let angled = false;
  for (let i = 0; i < value.length; i++) {
    const char = value[i]!;
    if (escaped) {
      escaped = false;
      continue;
    }
    if (quoted && char === "\\") {
      escaped = true;
      continue;
    }
    if (char === '"') quoted = !quoted;
    else if (!quoted && char === "<") angled = true;
    else if (!quoted && char === ">") angled = false;
    else if (!quoted && !angled && char === separator) {
      parts.push(value.slice(start, i));
      start = i + 1;
    }
  }
  parts.push(value.slice(start));
  return parts;
}
/** RFC 8288 Link relation matching with quoted-string and URI delimiters respected. */
export function nextLink(header: string | null): string | undefined {
  for (const value of splitOutsideQuotes(header ?? "", ",")) {
    const target = /^\s*<([^>]*)>/.exec(value);
    if (!target) continue;
    for (const parameter of splitOutsideQuotes(value.slice(target[0].length), ";")) {
      const match = /^\s*([^=\s]+)\s*=\s*(.*?)\s*$/.exec(parameter);
      if (!match || match[1]!.toLowerCase() !== "rel") continue;
      let relation = match[2]!;
      if (relation.startsWith('"')) {
        const quoted = /^"((?:\\.|[^"\\])*)"\s*$/.exec(relation);
        if (!quoted) continue;
        relation = quoted[1]!.replace(/\\(.)/g, "$1");
      } else if (!/^[^\s;,]+$/.test(relation)) continue;
      if (relation.split(/\s+/).some((v) => v.toLowerCase() === "next"))
        return target[1];
    }
  }
  return undefined;
}
