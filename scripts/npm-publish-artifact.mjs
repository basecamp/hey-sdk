#!/usr/bin/env node
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";

const [mode, packageName, version, tarball] = process.argv.slice(2);
if (!['--dry-run', '--publish'].includes(mode) || !packageName || !version || !tarball) {
  console.error("Usage: npm-publish-artifact.mjs <--dry-run|--publish> <package> <version> <tarball>");
  process.exit(2);
}
const registry = "https://registry.npmjs.org";
const run = (args) => spawnSync("npm", args, { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] });
const query = run([
  "view",
  `${packageName}@${version}`,
  "dist.integrity",
  "--json",
  `--registry=${registry}`,
]);
if (query.status === 0) {
  let actual;
  try {
    actual = JSON.parse(query.stdout.trim());
  } catch {}
  if (typeof actual !== "string") {
    console.error(query.stderr);
    console.error("Registry returned an invalid integrity value; refusing to continue.");
    process.exit(1);
  }
  const expected = `sha512-${createHash("sha512").update(readFileSync(tarball)).digest("base64")}`;
  if (actual !== expected) {
    console.error("Existing npm version differs from artifact.");
    process.exit(1);
  }
  console.log("Identical version already published; nothing to do.");
  process.exit(0);
}
let registryError;
try {
  registryError = JSON.parse(query.stdout.trim());
} catch {
  // npm --json writes its structured registry error to stdout. Human diagnostics
  // on stderr never establish that a version is absent.
}
if (
  !registryError ||
  typeof registryError !== "object" ||
  registryError.error?.code !== "E404"
) {
  const failure = `${query.stdout}\n${query.stderr}`.trim();
  if (failure) console.error(failure);
  console.error("Registry query failed; refusing to treat it as an unpublished version.");
  process.exit(1);
}
const args = [
  "publish",
  tarball,
  "--access",
  "public",
  "--tag",
  "latest",
  "--provenance",
  "--ignore-scripts",
  `--registry=${registry}`,
];
if (mode === "--dry-run") args.push("--dry-run");
const publish = run(args);
process.stdout.write(publish.stdout);
process.stderr.write(publish.stderr);
if (publish.status !== 0) process.exit(publish.status ?? 1);
