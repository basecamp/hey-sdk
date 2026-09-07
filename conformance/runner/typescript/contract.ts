import assert from "node:assert/strict";
const assertionTypes = new Set([
  "requestCount",
  "delayBetweenRequests",
  "noError",
  "errorCode",
  "errorField",
  "statusCode",
  "requestPath",
  "lastRequestPath",
  "requestMethod",
  "requestQuery",
  "lastRequestQuery",
  "requestBody",
  "requestForm",
  "lastRequestForm",
  "headerPresent",
  "lastRequestHeader",
  "responseMeta",
  "urlOrigin",
  "responseBody",
]);
const configKeys = new Set([
  "accountId",
  "baseUrl",
  "cacheEnabled",
  "clientLayer",
  "refreshableCredentials",
]);
export function validateFixture(tc: {
  name: string;
  assertions: { type: string; path?: string }[];
  configOverrides?: Record<string, unknown>;
}) {
  assert.ok(
    tc.name && Array.isArray(tc.assertions) && tc.assertions.length,
    "Fixture must have a name and assertions",
  );
  for (const a of tc.assertions) {
    if (!assertionTypes.has(a.type))
      throw new Error(`Unknown assertion: ${a.type}`);
    if (
      a.type === "errorField" &&
      !["httpStatus", "retryable", "requestId"].includes(a.path ?? "")
    )
      throw new Error(`Unknown error field: ${a.path}`);
    if (
      a.type === "responseMeta" &&
      !["totalCount", "nextPage"].includes(a.path ?? "")
    )
      throw new Error(`Unknown response metadata: ${a.path}`);
  }
  for (const key of Object.keys(tc.configOverrides ?? {}))
    if (!configKeys.has(key))
      throw new Error(`Unknown config override: ${key}`);
}
