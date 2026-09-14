import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { expect, it } from "vitest";

const { satisfies } = createRequire(import.meta.url)("semver") as {
  satisfies(version: string, range: string): boolean;
};
const read = (path: string) => JSON.parse(readFileSync(new URL(path, import.meta.url), "utf8"));
const manifest = read("../package.json");

it.each([
  ["22.11.0", false], ["22.11.99", false], ["22.12.0", true], ["22.99.0", true],
  ["20.20.0", false], ["23.0.0", false], ["23.99.0", false],
  ["24.0.0", true], ["24.20.0", true], ["24.99.0", true],
  ["25.0.0", false], ["25.99.0", false],
  ["26.0.0", true], ["26.7.0", true], ["26.99.0", true],
  ["27.0.0", false], ["28.0.0", false], ["30.0.0", false],
  ["24.0.0-rc.1", false],
])("declares support for Node %s: %s", (version, supported) => {
  expect(satisfies(version, manifest.engines.node)).toBe(supported);
});

it("keeps SDK and conformance frozen engine contracts synchronized", () => {
  expect(read("../package-lock.json").packages[""].engines).toEqual(manifest.engines);
  expect(read("../../conformance/runner/typescript/package.json").engines).toEqual(manifest.engines);
  expect(read("../../conformance/runner/typescript/package-lock.json").packages[""].engines).toEqual(manifest.engines);
});
