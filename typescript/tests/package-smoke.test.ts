import { mkdtempSync, readFileSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { execFileSync, spawnSync } from "node:child_process";
import { afterAll, beforeAll, expect, it } from "vitest";

const root = fileURLToPath(new URL("../", import.meta.url));
const temp = mkdtempSync(join(tmpdir(), "hey-package-smoke-test-"));
let tarball: string;
let files: string[];
let manifest: Record<string, unknown>;

beforeAll(() => {
  // Build/pack the real deliverable; malformed cases change only its manifest.
  execFileSync("npm", ["run", "build"], { cwd: root, stdio: "pipe" });
  const [pack] = JSON.parse(
    execFileSync("npm", ["pack", "--json", "--pack-destination", temp], {
      cwd: root,
      encoding: "utf8",
    }),
  );
  tarball = join(temp, pack.filename);
  files = pack.files.map((file: { path: string }) => `package/${file.path}`);
  execFileSync("tar", ["-xzf", tarball, "-C", temp]);
  manifest = JSON.parse(readFileSync(join(temp, "package/package.json"), "utf8"));
}, 30_000);

afterAll(() => rmSync(temp, { recursive: true, force: true }));

function smoke(artifact: string) {
  return spawnSync(process.execPath, [resolve(root, "scripts/package-smoke.mjs"), artifact], {
    encoding: "utf8",
    timeout: 20_000,
  });
}

function repack(field: string, value: unknown) {
  writeFileSync(join(temp, "package/package.json"), JSON.stringify({ ...manifest, [field]: value }));
  const artifact = join(temp, `${field}.tgz`);
  execFileSync("tar", ["-czf", artifact, "-C", temp, ...files]);
  return artifact;
}

it("installs and exercises the real packed artifact offline", () => {
  const result = smoke(tarball);
  expect(result.error).toBeUndefined();
  expect(result.status, result.stderr).toBe(0);
  expect(result.stdout).toContain("Package smoke passed");
}, 30_000);

it.each([
  ["optionalDependencies", { "lossless-json": "0.0.0" }],
  ["peerDependencies", { "hey-sdk-review-nonexistent-peer-9a6b735e": "1.0.0" }],
  ["peerDependenciesMeta", { "lossless-json": { optional: true } }],
  ["bundleDependencies", ["lossless-json"]],
  ["bundledDependencies", true],
  ["overrides", { "lossless-json": "0.0.0" }],
  ["workspaces", ["packages/*"]],
  ["os", ["!linux", "!darwin", "!win32"]],
  ["cpu", ["unsupported-review-cpu"]],
  ["libc", ["unsupported-review-libc"]],
])("rejects unsupported packed %s rather than testing a different install graph", (field, value) => {
  const result = smoke(repack(field as string, value));
  expect(result.error).toBeUndefined();
  expect(result.status).not.toBe(0);
  expect(result.stderr).toContain(`Unsupported package smoke manifest field: ${field}`);
});

it("rejects packed dependency drift from the frozen graph", () => {
  const result = smoke(repack("dependencies", { "lossless-json": "0.0.0" }));
  expect(result.status).not.toBe(0);
  expect(result.stderr).toContain("Packed dependencies differ from frozen lock");
});

it("rejects packed engine drift from the tested Node contract", () => {
  const result = smoke(repack("engines", { node: ">=999" }));
  expect(result.status).not.toBe(0);
  expect(result.stderr).toContain("Packed engines differ from frozen lock");
});
