import {
  mkdtempSync,
  cpSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
  rmSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { execFileSync } from "node:child_process";
import { it, expect } from "vitest";
it("bumps SDK manifests/constants/lock roots and syncs/checks API versions without touching generation", () => {
  const root = new URL("../../", import.meta.url),
    temp = mkdtempSync(join(tmpdir(), "hey-version-test-"));
  try {
    for (const file of [
      "scripts/lib.sh",
      "scripts/bump-version.sh",
      "scripts/sync-api-version.sh",
      "scripts/sync-typescript-versions.mjs",
      "go/pkg/hey/version.go",
      "typescript/src/version.ts",
      "typescript/package.json",
      "typescript/package-lock.json",
      "conformance/runner/typescript/package.json",
      "conformance/runner/typescript/package-lock.json",
      "openapi.json",
    ]) {
      const target = join(temp, file);
      mkdirSync(join(target, ".."), { recursive: true });
      cpSync(new URL(file, root), target);
    }
    execFileSync("bash", ["scripts/bump-version.sh", "1.2.3"], { cwd: temp });
    for (const dir of ["typescript", "conformance/runner/typescript"]) {
      expect(
        JSON.parse(readFileSync(join(temp, dir, "package.json"), "utf8"))
          .version,
      ).toBe("1.2.3");
      const lock = JSON.parse(
        readFileSync(join(temp, dir, "package-lock.json"), "utf8"),
      );
      expect(lock.version).toBe("1.2.3");
      expect(lock.packages[""].version).toBe("1.2.3");
    }
    expect(readFileSync(join(temp, "go/pkg/hey/version.go"), "utf8")).toContain(
      'const Version = "1.2.3"',
    );
    expect(
      readFileSync(join(temp, "typescript/src/version.ts"), "utf8"),
    ).toContain('const VERSION = "1.2.3"');
    const spec = JSON.parse(readFileSync(join(temp, "openapi.json"), "utf8"));
    spec.info.version = "2027-01-02";
    writeFileSync(join(temp, "openapi.json"), JSON.stringify(spec));
    execFileSync("bash", ["scripts/sync-api-version.sh"], { cwd: temp });
    execFileSync("bash", ["scripts/sync-api-version.sh", "--check"], {
      cwd: temp,
    });
    expect(
      readFileSync(join(temp, "typescript/src/version.ts"), "utf8"),
    ).toContain('const API_VERSION = "2027-01-02"');
    const manifest = join(temp, "typescript/package-lock.json"),
      before = readFileSync(manifest, "utf8");
    writeFileSync(
      manifest,
      before.replace('"version": "1.2.3"', '"version": "9.9.9"'),
    );
    expect(() =>
      execFileSync(
        process.execPath,
        ["scripts/sync-typescript-versions.mjs", "--check"],
        { cwd: temp, stdio: "pipe" },
      ),
    ).toThrow();
  } finally {
    rmSync(temp, { recursive: true, force: true });
  }
});
