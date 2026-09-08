import {
  mkdtempSync,
  cpSync,
  chmodSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
  rmSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { delimiter, join } from "node:path";
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
      "rust/hey-sdk/Cargo.toml",
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
    const bin = join(temp, "bin");
    mkdirSync(bin);
    const cargo = join(bin, "cargo");
    writeFileSync(cargo, "#!/bin/sh\nexit 0\n");
    chmodSync(cargo, 0o755);
    mkdirSync(join(temp, "conformance/runner/rust"), { recursive: true });
    const bumpEnvironment = {
      ...process.env,
      PATH: `${bin}${delimiter}${process.env.PATH}`,
    };
    const versionFiles = [
      "go/pkg/hey/version.go",
      "rust/hey-sdk/Cargo.toml",
      "typescript/src/version.ts",
      "typescript/package.json",
      "typescript/package-lock.json",
      "conformance/runner/typescript/package.json",
      "conformance/runner/typescript/package-lock.json",
    ];
    const snapshot = () =>
      versionFiles.map((file) => readFileSync(join(temp, file), "utf8"));
    for (const invalid of ["", "01.2.3", "1.02.3", "1.2.03", "v1.2.3", "1.2.3-beta"]) {
      const before = snapshot();
      expect(() =>
        execFileSync("bash", ["scripts/bump-version.sh", invalid], {
          cwd: temp,
          env: bumpEnvironment,
          stdio: "pipe",
        }),
      ).toThrow();
      expect(snapshot()).toEqual(before);
      expect(() =>
        execFileSync(
          process.execPath,
          ["scripts/sync-typescript-versions.mjs", "--sdk-version", invalid],
          { cwd: temp, stdio: "pipe" },
        ),
      ).toThrow();
      expect(snapshot()).toEqual(before);
    }

    const lateLock = join(
      temp,
      "conformance/runner/typescript/package-lock.json",
    );
    const validLateLock = readFileSync(lateLock, "utf8");
    writeFileSync(lateLock, "{ malformed");
    const beforeMalformedTarget = snapshot();
    expect(() =>
      execFileSync("bash", ["scripts/bump-version.sh", "2.3.4"], {
        cwd: temp,
        env: bumpEnvironment,
        stdio: "pipe",
      }),
    ).toThrow();
    expect(snapshot()).toEqual(beforeMalformedTarget);
    writeFileSync(lateLock, validLateLock);

    const openapiPath = join(temp, "openapi.json");
    const validOpenAPI = readFileSync(openapiPath, "utf8");
    const invalidSpec = JSON.parse(validOpenAPI);
    invalidSpec.info.version = "not-a-date";
    writeFileSync(openapiPath, JSON.stringify(invalidSpec));
    const beforeInvalidAPI = snapshot();
    expect(() =>
      execFileSync("bash", ["scripts/sync-api-version.sh"], {
        cwd: temp,
        stdio: "pipe",
      }),
    ).toThrow();
    expect(snapshot()).toEqual(beforeInvalidAPI);
    writeFileSync(openapiPath, validOpenAPI);

    const goVersionPath = join(temp, "go/pkg/hey/version.go");
    const validGoVersion = readFileSync(goVersionPath, "utf8");
    writeFileSync(
      goVersionPath,
      validGoVersion.replace("const Version =", "const MissingVersion ="),
    );
    expect(() =>
      execFileSync(
        process.execPath,
        ["scripts/sync-typescript-versions.mjs", "--check"],
        { cwd: temp, stdio: "pipe" },
      ),
    ).toThrow();
    writeFileSync(goVersionPath, validGoVersion);

    const missingInfo = JSON.parse(validOpenAPI);
    delete missingInfo.info.version;
    writeFileSync(openapiPath, JSON.stringify(missingInfo));
    expect(() =>
      execFileSync(
        process.execPath,
        ["scripts/sync-typescript-versions.mjs", "--check"],
        { cwd: temp, stdio: "pipe" },
      ),
    ).toThrow();
    writeFileSync(openapiPath, validOpenAPI);

    execFileSync("bash", ["scripts/bump-version.sh", "0.0.0"], {
      cwd: temp,
      env: bumpEnvironment,
    });
    expect(JSON.parse(readFileSync(join(temp, "typescript/package.json"), "utf8")).version).toBe("0.0.0");
    execFileSync("bash", ["scripts/bump-version.sh", "1.2.3"], {
      cwd: temp,
      env: bumpEnvironment,
    });
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
    expect(readFileSync(join(temp, "rust/hey-sdk/Cargo.toml"), "utf8")).toMatch(
      /^version = "1\.2\.3"$/m,
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
