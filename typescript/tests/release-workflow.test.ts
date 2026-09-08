import { createHash } from "node:crypto";
import {
  chmodSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { delimiter, join } from "node:path";
import { execFileSync } from "node:child_process";
import { it, expect } from "vitest";

it("uses one versioned publication decision for publishing and orchestration", () => {
  const state = readFileSync(
    new URL("../../.github/typescript-publish-enabled", import.meta.url),
    "utf8",
  ).trim();
  const typescriptWorkflow = readFileSync(
    new URL("../../.github/workflows/release-typescript.yml", import.meta.url),
    "utf8",
  );
  const githubWorkflow = readFileSync(
    new URL("../../.github/workflows/release-github.yml", import.meta.url),
    "utf8",
  );
  expect(["true", "false"]).toContain(state);
  expect(typescriptWorkflow).toContain("needs.build.outputs.publish_enabled == 'true'");
  expect(typescriptWorkflow).toContain("bash ../scripts/typescript-publish-state.sh");
  expect(githubWorkflow).toContain("bash scripts/typescript-publish-state.sh");
  expect(githubWorkflow).toContain("steps.activation.outputs.enabled == 'true'");
  expect(`${typescriptWorkflow}\n${githubWorkflow}`).not.toContain(
    "HEY_TYPESCRIPT_PUBLISH_ENABLED",
  );
});

it("rejects malformed versioned publication state", () => {
  const temp = mkdtempSync(join(tmpdir(), "hey-publish-state-"));
  try {
    const state = join(temp, "state");
    writeFileSync(state, "enabled\n");
    expect(() =>
      execFileSync(
        "bash",
        [
          new URL("../../scripts/typescript-publish-state.sh", import.meta.url)
            .pathname,
          state,
        ],
        { stdio: "pipe" },
      ),
    ).toThrow();
  } finally {
    rmSync(temp, { recursive: true, force: true });
  }
});

it("queries public npm integrity and only skips identical artifact bytes", () => {
  const temp = mkdtempSync(join(tmpdir(), "hey-npm-release-"));
  try {
    const tarball = join(temp, "37signals-hey-1.2.3.tgz");
    const calls = join(temp, "calls");
    writeFileSync(tarball, "tested artifact bytes");
    const integrity = `sha512-${createHash("sha512").update(readFileSync(tarball)).digest("base64")}`;
    const npm = join(temp, "npm");
    writeFileSync(npm, `#!/bin/sh
echo "$@" >> "$MOCK_CALLS"
if [ "$1" = view ]; then
  case "$MOCK_RESULT" in
    identical) printf '%s\\n' '"${integrity}"'; exit 0 ;;
    conflict) printf '%s\\n' '"sha512-conflict"'; exit 0 ;;
    absent) echo '{"error":{"code":"E404"}}'; exit 1 ;;
    failure) echo 'npm error code E500' >&2; exit 1 ;;
    incidental) echo '{"error":{"code":"E500","summary":"upstream mentioned E404"}}'; exit 1 ;;
    malformed) echo 'npm error code E404'; exit 1 ;;
  esac
fi
exit 0
`);
    chmodSync(npm, 0o755);
    const script = new URL("../../scripts/npm-publish-artifact.mjs", import.meta.url);
    const run = (mode: "--dry-run" | "--publish", result: string) =>
      execFileSync(process.execPath, [script.pathname, mode, "@37signals/hey", "1.2.3", tarball], {
        encoding: "utf8",
        env: {
          ...process.env,
          PATH: `${temp}${delimiter}${process.env.PATH}`,
          MOCK_CALLS: calls,
          MOCK_RESULT: result,
        },
        stdio: "pipe",
      });
    expect(run("--dry-run", "identical")).toContain("nothing to do");
    expect(readFileSync(calls, "utf8")).toContain("--registry=https://registry.npmjs.org");
    expect(readFileSync(calls, "utf8")).not.toContain("publish ");
    for (const result of ["conflict", "failure", "incidental", "malformed"]) {
      writeFileSync(calls, "");
      expect(() => run("--dry-run", result)).toThrow();
      expect(readFileSync(calls, "utf8")).not.toContain("publish ");
    }
    writeFileSync(calls, "");
    run("--dry-run", "absent");
    expect(readFileSync(calls, "utf8")).toMatch(/publish .*--dry-run/);
    writeFileSync(calls, "");
    run("--publish", "absent");
    expect(readFileSync(calls, "utf8")).toContain("publish ");
    expect(readFileSync(calls, "utf8")).not.toContain("--dry-run");
  } finally {
    rmSync(temp, { recursive: true, force: true });
  }
});
