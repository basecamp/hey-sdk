import assert from "node:assert/strict";
import { stringifyJSON } from "../../../typescript/src/security.js";

/** Fixture null and omission both represent a response without body bytes. */
export function serializeMockResponseBody(value: unknown): string | undefined {
  return value === null || value === undefined ? undefined : stringifyJSON(value);
}

function lookup(value: unknown, path: string): { present: boolean; value?: unknown } {
  let current = value;
  for (const key of path.split(".")) {
    if (
      current === null ||
      (typeof current !== "object" && typeof current !== "function") ||
      !Object.hasOwn(current, key)
    )
      return { present: false };
    current = (current as Record<string, unknown>)[key];
  }
  return { present: true, value: current };
}

/** requestBody null means the path must be absent, matching the shared Go contract. */
export function assertBodyFields(
  actual: unknown,
  expected: Record<string, unknown>,
): void {
  for (const [path, wanted] of Object.entries(expected)) {
    const found = lookup(actual, path);
    if (wanted === null)
      assert.equal(found.present, false, `Expected request body path ${path} to be absent`);
    else {
      assert.equal(found.present, true, `Expected request body path ${path} to be present`);
      assert.deepEqual(found.value, wanted);
    }
  }
}
