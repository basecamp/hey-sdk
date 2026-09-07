#!/usr/bin/env node
// Update only explicit requested versions; --check verifies every shipped TS artifact.
import { readFileSync, writeFileSync } from "node:fs";
const root = new URL("../", import.meta.url);
const args = process.argv.slice(2);
const check = args.includes("--check");
const argument = (name) => {
  const i = args.indexOf(name);
  return i < 0 ? undefined : args[i + 1];
};
const go = readFileSync(new URL("go/pkg/hey/version.go", root), "utf8");
const sdk =
  argument("--sdk-version") ??
  (check ? /^const Version = "([^"]+)"/m.exec(go)?.[1] : undefined);
const api =
  argument("--api-version") ??
  (check
    ? JSON.parse(readFileSync(new URL("openapi.json", root), "utf8")).info
        .version
    : undefined);
if (sdk && !/^\d+\.\d+\.\d+$/.test(sdk)) throw new Error("Invalid SDK version");
if (api && !/^\d{4}-\d{2}-\d{2}$/.test(api))
  throw new Error("Invalid API version");
function update(path, transform) {
  const file = new URL(path, root),
    before = readFileSync(file, "utf8"),
    after = transform(before);
  if (check) {
    if (before !== after) throw new Error(`Version drift: ${path}`);
  } else if (before !== after) writeFileSync(file, after);
}
update("typescript/src/version.ts", (text) => {
  for (const [key, value] of [
    ["VERSION", sdk],
    ["API_VERSION", api],
  ])
    if (value) {
      const pattern = new RegExp(`^export const ${key} = "[^"]+";`, "m");
      if (!pattern.test(text)) throw new Error(`Missing ${key}`);
      text = text.replace(pattern, `export const ${key} = "${value}";`);
    }
  return text;
});
if (sdk)
  for (const dir of ["typescript", "conformance/runner/typescript"])
    for (const name of ["package.json", "package-lock.json"])
      update(`${dir}/${name}`, (text) => {
        const json = JSON.parse(text);
        json.version = sdk;
        if (name === "package-lock.json") json.packages[""].version = sdk;
        return JSON.stringify(json, null, 2) + "\n";
      });
console.log(
  `TypeScript versions ${check ? "verified" : "synced"}${sdk ? ` SDK ${sdk}` : ""}${api ? ` API ${api}` : ""}`,
);
