// Install the packed artifact outside the repository: never resolve local src/dist.
import { mkdtempSync, writeFileSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve, dirname } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { execFileSync } from "node:child_process";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const temp = mkdtempSync(join(tmpdir(), "hey-npm-smoke-"));
try {
  const supplied = process.argv[2] ? resolve(root, process.argv[2]) : undefined;
  const [pack] = supplied
    ? [
        {
          filename: supplied,
          files: execFileSync("tar", ["-tzf", supplied], { encoding: "utf8" })
            .trim()
            .split("\n")
            .map((path) => ({ path: path.replace(/^package\//, "") })),
        },
      ]
    : JSON.parse(
        execFileSync("npm", ["pack", "--json", "--pack-destination", temp], {
          cwd: root,
          encoding: "utf8",
        }),
      );
  const tarball = supplied ?? join(temp, pack.filename);
  const paths = pack.files.map((f) => f.path);
  for (const file of [
    "dist/index.js",
    "dist/index.d.ts",
    "dist/oauth.js",
    "dist/oauth.d.ts",
    "dist/generated/schema.d.ts",
    "README.md",
    "LICENSE",
  ])
    assert(paths.includes(file), `Missing ${file}`);
  assert(
    paths.every(
      (p) =>
        p.startsWith("dist/") ||
        ["package.json", "README.md", "LICENSE"].includes(p),
    ),
    "Unexpected package file",
  );
  assert(
    !paths.some((p) => p.includes("node_modules") || p.endsWith(".test.js")),
  );
  // npm ci caches locked tarballs, not registry metadata. Seed a consumer lock
  // with the frozen production graph so this install also works on a cold CI
  // runner without network access or a developer's warmed metadata cache.
  const packedManifest = JSON.parse(
    execFileSync("tar", ["-xOf", tarball, "package/package.json"], {
      encoding: "utf8",
    }),
  );
  const lock = JSON.parse(readFileSync(join(root, "package-lock.json"), "utf8"));
  assert.deepEqual(packedManifest.dependencies, lock.packages[""].dependencies);
  const consumer = {
    private: true,
    type: "module",
    dependencies: { [packedManifest.name]: pathToFileURL(tarball).href },
  };
  const packages = Object.fromEntries(
    Object.entries(lock.packages).filter(([path, entry]) => path && !entry.dev),
  );
  packages[""] = consumer;
  packages[`node_modules/${packedManifest.name}`] = {
    version: packedManifest.version,
    resolved: pathToFileURL(tarball).href,
    integrity: `sha512-${createHash("sha512").update(readFileSync(tarball)).digest("base64")}`,
    dependencies: packedManifest.dependencies,
  };
  writeFileSync(join(temp, "package.json"), JSON.stringify(consumer));
  writeFileSync(
    join(temp, "package-lock.json"),
    JSON.stringify({ lockfileVersion: 3, requires: true, packages }),
  );
  execFileSync(
    "npm",
    ["ci", "--ignore-scripts", "--offline", "--no-audit", "--no-fund"],
    { cwd: temp, stdio: "pipe" },
  );
  const manifest = JSON.parse(
    readFileSync(
      join(temp, "node_modules/@37signals/hey/package.json"),
      "utf8",
    ),
  );
  assert.equal(manifest.name, "@37signals/hey");
  writeFileSync(
    join(temp, "smoke.mjs"),
    `import assert from 'node:assert/strict';
import {HeyClient, VERSION, API_VERSION} from '@37signals/hey';
import {generatePKCE} from '@37signals/hey/oauth';
assert.equal(VERSION, ${JSON.stringify(manifest.version)});
assert.ok(API_VERSION); assert.equal(generatePKCE().verifier.length,43);
const c=new HeyClient({token:'hermetic',fetch:async(url,init)=>{
assert.equal(new URL(url).pathname,'/boxes/9007199254740993');
assert.equal(new Headers(init.headers).get('Authorization'),'Bearer hermetic');
return new Response('{"id":9007199254740993}');
}});
assert.equal((await c.getBox({path:{boxId:9007199254740993n}})).data.id,9007199254740993n);
`,
  );
  execFileSync(process.execPath, ["smoke.mjs"], {
    cwd: temp,
    stdio: "inherit",
  });
  writeFileSync(
    join(temp, "smoke.ts"),
    `import {HeyClient, type OperationInput} from '@37signals/hey';
import {generatePKCE} from '@37signals/hey/oauth';
const c=new HeyClient({token:'test'});
const input: OperationInput<'GetBox'>={path:{boxId:9007199254740993n}};
void c.getBox(input); generatePKCE();
// @ts-expect-error required path must not disappear
void c.getBox({});
// @ts-expect-error unknown operations must not become any
void c.notAHeyOperation();
`,
  );
  execFileSync(
    process.execPath,
    [
      join(root, "node_modules/typescript/bin/tsc"),
      "--strict",
      "--noEmit",
      "--module",
      "NodeNext",
      "--target",
      "ES2022",
      "--lib",
      "ES2022,DOM,DOM.Iterable",
      "smoke.ts",
    ],
    { cwd: temp, stdio: "inherit" },
  );
  console.log(
    `Package smoke passed: ${manifest.name}@${manifest.version}, ${paths.length} files, isolated ESM imports + OAuth + int64 + consumer typecheck`,
  );
} finally {
  rmSync(temp, { recursive: true, force: true });
}
