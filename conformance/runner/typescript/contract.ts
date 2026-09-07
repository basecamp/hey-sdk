import assert from "node:assert/strict";

type Check = (value: unknown) => boolean;
const record = (value: unknown): value is Record<string, unknown> =>
  value !== null && typeof value === "object" && !Array.isArray(value);
const text: Check = value => typeof value === "string";
const nonempty: Check = value => text(value) && (value as string).length > 0;
const nonnegative: Check = value => typeof value === "number" && Number.isFinite(value) && value >= 0;
const integer: Check = value => nonnegative(value) && Number.isSafeInteger(value);
const scalar: Check = value => value === null || text(value) || typeof value === "boolean" ||
  typeof value === "bigint" || (typeof value === "number" && Number.isFinite(value));
// Fixture JSON is parsed losslessly, so large integer literals arrive as bigint.
const json: Check = value => scalar(value) || (Array.isArray(value)
  ? value.every(json) : record(value) && Object.values(value).every(json));
const fields = (check: Check): Check => value => record(value) && Object.values(value).every(check);
const shapes: Record<string, { required: Record<string, Check>; optional?: Record<string, Check> }> = {
  requestCount: { required: { expected: integer } },
  delayBetweenRequests: { required: {}, optional: { min: nonnegative, max: nonnegative } },
  noError: { required: {} },
  errorCode: { required: { expected: nonempty } },
  errorField: { required: { path: nonempty, expected: scalar } },
  statusCode: { required: { expected: integer } },
  requestPath: { required: { expected: nonempty } },
  lastRequestPath: { required: { expected: nonempty } },
  requestMethod: { required: { expected: nonempty } },
  requestQuery: { required: { expected: fields(scalar) } },
  lastRequestQuery: { required: { expected: fields(scalar) } },
  requestBody: { required: { expected: fields(json) } },
  requestForm: { required: { expected: fields(scalar) } },
  lastRequestForm: { required: { expected: fields(scalar) } },
  headerPresent: { required: { path: nonempty } },
  lastRequestHeader: { required: { path: nonempty, expected: text } },
  responseMeta: { required: { path: nonempty, expected: scalar } },
  urlOrigin: { required: { expected: value => value === "rejected" } },
  responseBody: { required: { path: nonempty, expected: json } },
};
const errorFields: Record<string, Check> = {
  httpStatus: integer, retryable: value => typeof value === "boolean", requestId: text,
};
const responseMetadata: Record<string, Check> = { totalCount: integer, nextPage: text };
const configKeys = new Set([
  "accountId",
  "baseUrl",
  "cacheEnabled",
  "clientLayer",
  "refreshableCredentials",
]);
export function validateFixture(tc: {
  name: string;
  assertions: unknown[];
  configOverrides?: Record<string, unknown>;
}) {
  assert.ok(
    tc.name && Array.isArray(tc.assertions) && tc.assertions.length,
    "Fixture must have a name and assertions",
  );
  for (const a of tc.assertions) {
    assert.ok(record(a) && Object.hasOwn(a, "type") && typeof a.type === "string", "Assertion must have its own type");
    if (!Object.hasOwn(shapes, a.type))
      throw new Error(`Unknown assertion: ${a.type}`);
    const shape = shapes[a.type]!;
    const allowed = { ...shape.required, ...shape.optional };
    for (const key of Object.keys(a)) {
      if (key === "type") continue;
      assert.ok(Object.hasOwn(allowed, key), `Unknown ${a.type} assertion field: ${key}`);
      assert.ok(allowed[key]!(a[key]), `Invalid ${a.type} assertion field: ${key}`);
    }
    for (const key of Object.keys(shape.required))
      assert.ok(Object.hasOwn(a, key), `Missing ${a.type} assertion field: ${key}`);
    if (a.type === "delayBetweenRequests") {
      assert.ok(Object.hasOwn(a, "min") || Object.hasOwn(a, "max"), "Delay assertion requires min or max");
      if (Object.hasOwn(a, "min") && Object.hasOwn(a, "max"))
        assert.ok((a.min as number) <= (a.max as number), "Delay min exceeds max");
    }
    if (a.type === "errorField" || a.type === "responseMeta") {
      const paths = a.type === "errorField" ? errorFields : responseMetadata;
      const path = a.path as string;
      assert.ok(Object.hasOwn(paths, path), `Unknown ${a.type} path: ${path}`);
      assert.ok(paths[path]!(a.expected), `Invalid ${a.type} expected value for ${path}`);
    }
  }
  for (const key of Object.keys(tc.configOverrides ?? {}))
    if (!configKeys.has(key))
      throw new Error(`Unknown config override: ${key}`);
}
