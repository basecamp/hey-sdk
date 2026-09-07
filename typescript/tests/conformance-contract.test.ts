import { it, expect } from "vitest";
import {
  repetitionCount,
  validateFixture,
} from "../../conformance/runner/typescript/contract.js";
import { assertBodyFields } from "../../conformance/runner/typescript/assertions.js";
const legitimate: Record<string, unknown>[] = [
  { type: "requestCount", expected: 0 },
  { type: "delayBetweenRequests", min: 0 },
  { type: "delayBetweenRequests", max: 2000 },
  { type: "delayBetweenRequests", min: 1000, max: 2000 },
  { type: "noError" },
  { type: "errorCode", expected: "auth_required" },
  { type: "errorField", path: "httpStatus", expected: 401 },
  { type: "errorField", path: "retryable", expected: false },
  { type: "errorField", path: "requestId", expected: "" },
  { type: "statusCode", expected: 200 },
  { type: "requestPath", expected: "/identity.json" },
  { type: "lastRequestPath", expected: "/identity.json" },
  { type: "requestMethod", expected: "GET" },
  ...["requestQuery", "lastRequestQuery", "requestForm", "lastRequestForm"].map(type => ({
    type, expected: { absent: null, text: "", count: 1, enabled: true, id: 9007199254740993n },
  })),
  { type: "requestBody", expected: { absent: null, nested: { list: [true, 9007199254740993n] } } },
  { type: "headerPresent", path: "Authorization" },
  { type: "lastRequestHeader", path: "If-None-Match", expected: "" },
  { type: "responseMeta", path: "totalCount", expected: 42 },
  { type: "responseMeta", path: "nextPage", expected: "cursor" },
  { type: "urlOrigin", expected: "rejected" },
  ...[null, false, 42, "", 9007199254740993n, [1], { nested: [null] }].map(expected => ({
    type: "responseBody", path: "data", expected,
  })),
];
function validate(assertion: unknown) {
  validateFixture({ name: "shape test", assertions: [assertion as { type: string }] });
}
it.each(legitimate)("accepts legitimate assertion %# $type", assertion => {
  expect(() => validate(assertion)).not.toThrow();
});
it.each(legitimate)("rejects extra, missing and inherited fields for assertion %# $type", assertion => {
  expect(() => validate({ ...assertion, expectd: "cursor" })).toThrow();
  for (const key of Object.keys(assertion)) {
    if (assertion.type === "delayBetweenRequests" && Object.hasOwn(assertion, "min") && Object.hasOwn(assertion, "max") && key !== "type") continue;
    const missing = { ...assertion };
    delete missing[key];
    expect(() => validate(missing), `missing ${key}`).toThrow();
    expect(() => validate(Object.assign(Object.create({ [key]: assertion[key] }), missing)), `inherited ${key}`).toThrow();
  }
});
it.each([
  null, [], {}, { type: 1 },
  { type: "responseMeta", path: "nextPage", expectd: "cursor" },
  ...["requestCount", "statusCode"].flatMap(type => ["1", -1, 1.5, NaN, Infinity].map(expected => ({ type, expected }))),
  { type: "delayBetweenRequests" },
  { type: "delayBetweenRequests", min: "100" },
  { type: "delayBetweenRequests", max: -1 },
  { type: "delayBetweenRequests", min: 2, max: 1 },
  { type: "delayBetweenRequests", min: undefined },
  { type: "errorCode", expected: 401 },
  ...["requestPath", "lastRequestPath", "requestMethod"].map(type => ({ type, expected: 1 })),
  { type: "errorField", path: "httpStatus", expected: "401" },
  { type: "errorField", path: "retryable", expected: "false" },
  { type: "errorField", path: "requestId", expected: 1 },
  { type: "responseMeta", path: "totalCount", expected: "42" },
  { type: "responseMeta", path: "nextPage", expected: 1 },
  ...["requestQuery", "lastRequestQuery", "requestForm", "lastRequestForm", "requestBody"].flatMap(type =>
    [null, [], "fields", { field: undefined }].map(expected => ({ type, expected }))),
  { type: "requestQuery", expected: { field: {} } },
  { type: "headerPresent", path: 1 },
  { type: "headerPresent", path: "" },
  { type: "lastRequestHeader", path: "Authorization", expected: false },
  { type: "urlOrigin", expected: "allowed" },
  { type: "responseBody", path: 1, expected: null },
  { type: "responseBody", path: "id", expected: undefined },
  { type: "responseBody", path: "id", expected: { nested: undefined } },
])("rejects malformed assertion %#", assertion => {
  expect(() => validate(assertion)).toThrow();
});

it("treats null request-body expectations as absence rather than present null", () => {
  expect(() => assertBodyFields({ kept: 1 }, { absent: null, kept: 1 })).not.toThrow();
  expect(() => assertBodyFields({ absent: null }, { absent: null })).toThrow();
});

it("validates configuration domains and shares zero-means-once repetition", () => {
  const assertion = [{ type: "noError" }];
  for (const configOverrides of [
    { accountId: 0 },
    { accountId: 1.5 },
    { accountId: Number.MAX_SAFE_INTEGER + 1 },
    { accountId: "42" },
    { baseUrl: 1 },
    { baseUrl: "not a URL" },
    { cacheEnabled: 1 },
    { refreshableCredentials: "true" },
    { clientLayer: "generated" },
  ])
    expect(() =>
      validateFixture({ name: "test", assertions: assertion, configOverrides }),
    ).toThrow();
  for (const configOverrides of [
    { accountId: 42 },
    { accountId: 9007199254740993n },
    { baseUrl: "http://evil.example.com" },
    { cacheEnabled: false, refreshableCredentials: true, clientLayer: "hey" },
  ])
    expect(() =>
      validateFixture({ name: "test", assertions: assertion, configOverrides }),
    ).not.toThrow();
  for (const repeatOperation of [-1, 1.5, Number.MAX_SAFE_INTEGER + 1, "1"])
    expect(() =>
      validateFixture({ name: "test", assertions: assertion, repeatOperation }),
    ).toThrow();
  expect(repetitionCount(0)).toBe(1);
  expect(repetitionCount(2)).toBe(2);
});

it("fails closed on unknown assertions, fields, configurations and empty assertions even for exclusions", () => {
  for (const assertions of [
    [],
    [{ type: "unknown" }],
    [{ type: "errorField", path: "invented" }],
    [{ type: "responseMeta", path: "invented" }],
  ])
    expect(() => validateFixture({ name: "test", assertions })).toThrow();
  expect(() =>
    validateFixture({
      name: "test",
      assertions: [{ type: "noError" }],
      configOverrides: { unknown: true },
    }),
  ).toThrow();
  expect(() =>
    validateFixture({ name: "test", assertions: [{ type: "noError" }] }),
  ).not.toThrow();
});
