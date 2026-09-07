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
export async function readBounded(
  response: Response,
  max: number,
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
        await reader.cancel();
        throw new HeyError(
          "response_too_large",
          `Response exceeds ${max} bytes`,
          response.status,
          false,
          response.headers.get("X-Request-Id") ?? "",
        );
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }
  return Buffer.concat(chunks).toString("utf8");
}
/** RFC Link relation matching; commas in URLs are not separators. */
export function nextLink(header: string | null): string | undefined {
  for (const match of (header ?? "").matchAll(/<([^>]*)>([^<]*)/g)) {
    for (const part of match[2]!.split(";")) {
      const rel = /^\s*rel\s*=\s*(?:"([^"]*)"|([^,\s]+))/i.exec(part);
      if (
        (rel?.[1] ?? rel?.[2] ?? "")
          .split(/\s+/)
          .some((v) => v.toLowerCase() === "next")
      )
        return match[1];
    }
  }
  return undefined;
}
