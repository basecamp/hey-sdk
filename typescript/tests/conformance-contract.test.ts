import { it, expect } from "vitest";
import { validateFixture } from "../../conformance/runner/typescript/contract.js";
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
