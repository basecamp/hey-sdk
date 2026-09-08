#!/usr/bin/env node
// Atomically update explicit versions; --check verifies every shipped TS artifact.
import {
  existsSync,
  readFileSync,
  renameSync,
  rmSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { fileURLToPath } from "node:url";

const root = new URL("../", import.meta.url);
const args = process.argv.slice(2);
const check = args.includes("--check");
function argument(name) {
  const index = args.indexOf(name);
  if (index < 0) return undefined;
  const value = args[index + 1];
  if (value === undefined || value.startsWith("--"))
    throw new Error(`Missing value for ${name}`);
  return value;
}
const requestedSDK = argument("--sdk-version");
const requestedAPI = argument("--api-version");
if (!check && requestedSDK === undefined && requestedAPI === undefined)
  throw new Error("Specify --check, --sdk-version, or --api-version");

const goFile = new URL("go/pkg/hey/version.go", root);
const go = readFileSync(goFile, "utf8");
const sdk =
  requestedSDK ??
  (check
    ? /^const Version = "([^"]+)"$/m.exec(go)?.[1]
    : undefined);
let api = requestedAPI;
if (api === undefined && check) {
  const spec = JSON.parse(readFileSync(new URL("openapi.json", root), "utf8"));
  api = spec?.info?.version;
}
if (check && sdk === undefined)
  throw new Error("Could not read the Go SDK version");
if (check && api === undefined)
  throw new Error("Could not read the OpenAPI version");
if (
  sdk !== undefined &&
  !/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(sdk)
)
  throw new Error("Invalid SDK version");
if (api !== undefined && !/^\d{4}-\d{2}-\d{2}$/.test(api))
  throw new Error("Invalid API version");

const plans = [];
function plan(path, transform) {
  const file = fileURLToPath(new URL(path, root));
  const before = readFileSync(file, "utf8");
  plans.push({ file, before, after: transform(before), mode: statSync(file).mode });
}
function replaceConstant(text, name, value) {
  if (value === undefined) return text;
  const pattern = new RegExp(`^const ${name} = "[^"]+"$`, "m");
  if (!pattern.test(text)) throw new Error(`Missing Go ${name}`);
  return text.replace(pattern, `const ${name} = "${value}"`);
}
plan("go/pkg/hey/version.go", (text) => {
  text = replaceConstant(text, "Version", sdk);
  return replaceConstant(text, "APIVersion", api);
});
plan("typescript/src/version.ts", (text) => {
  for (const [key, value] of [
    ["VERSION", sdk],
    ["API_VERSION", api],
  ]) {
    if (value === undefined) continue;
    const pattern = new RegExp(`^export const ${key} = "[^"]+";`, "m");
    if (!pattern.test(text)) throw new Error(`Missing ${key}`);
    text = text.replace(pattern, `export const ${key} = "${value}";`);
  }
  return text;
});
if (sdk !== undefined)
  for (const dir of ["typescript", "conformance/runner/typescript"])
    for (const name of ["package.json", "package-lock.json"])
      plan(`${dir}/${name}`, (text) => {
        const json = JSON.parse(text);
        if (!json || typeof json !== "object")
          throw new Error(`Invalid manifest: ${dir}/${name}`);
        json.version = sdk;
        if (name === "package-lock.json") {
          if (!json.packages?.[""])
            throw new Error(`Invalid lockfile root: ${dir}/${name}`);
          json.packages[""].version = sdk;
        }
        return JSON.stringify(json, null, 2) + "\n";
      });

if (check) {
  for (const { file, before, after } of plans)
    if (before !== after) throw new Error(`Version drift: ${file}`);
} else {
  const changed = plans.filter(({ before, after }) => before !== after);
  const nonce = `${process.pid}-${Date.now()}`;
  for (const [index, item] of changed.entries()) {
    item.temporary = `${item.file}.tmp-${nonce}-${index}`;
    item.backup = `${item.file}.bak-${nonce}-${index}`;
  }
  try {
    // Compute and stage every replacement before moving any original.
    for (const item of changed)
      writeFileSync(item.temporary, item.after, { mode: item.mode });
    for (const item of changed) renameSync(item.file, item.backup);
    for (const item of changed) renameSync(item.temporary, item.file);
  } catch (error) {
    for (const item of changed)
      if (existsSync(item.backup)) {
        rmSync(item.file, { force: true });
        renameSync(item.backup, item.file);
      }
    for (const item of changed)
      if (existsSync(item.temporary)) rmSync(item.temporary, { force: true });
    throw error;
  }
  for (const item of changed) rmSync(item.backup, { force: true });
}

console.log(
  `TypeScript versions ${check ? "verified" : "synced"}${sdk !== undefined ? ` SDK ${sdk}` : ""}${api !== undefined ? ` API ${api}` : ""}`,
);
