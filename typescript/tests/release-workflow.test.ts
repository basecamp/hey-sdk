import { createHash } from "node:crypto";
import {
  chmodSync,
  copyFileSync,
  mkdirSync,
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
  expect(typescriptWorkflow).toContain(
    "enabled=$(bash ../scripts/typescript-publish-state.sh)",
  );
  expect(typescriptWorkflow).toContain(
    'echo "enabled=$enabled" >> "$GITHUB_OUTPUT"',
  );
  expect(githubWorkflow).toContain("ref: ${{ github.workflow_sha }}");
  expect(githubWorkflow).toContain("scripts/release-languages.sh");
  expect(githubWorkflow).toContain(
    "languages: ${{ steps.activation.outputs.languages }}",
  );
  expect(`${typescriptWorkflow}\n${githubWorkflow}`).not.toContain(
    "HEY_TYPESCRIPT_PUBLISH_ENABLED",
  );
});

it("selects current and historical release languages without weakening state validation", () => {
  const temp = mkdtempSync(join(tmpdir(), "hey-release-languages-"));
  const selector = new URL("../../scripts/release-languages.sh", import.meta.url).pathname;
  const stateHelper = new URL("../../scripts/typescript-publish-state.sh", import.meta.url).pathname;
  let sequence = 0;
  const target = (setup: (root: string) => void, event = "workflow_dispatch") => {
    const root = join(temp, String(sequence++));
    mkdirSync(join(root, ".github", "workflows"), { recursive: true });
    mkdirSync(join(root, "scripts"), { recursive: true });
    setup(root);
    return () => execFileSync("bash", [selector, root, event], {
      encoding: "utf8", stdio: "pipe",
    }).trim();
  };
  const current = (state: string) => target(root => {
    writeFileSync(join(root, ".github", "typescript-publish-enabled"), state);
    copyFileSync(stateHelper, join(root, "scripts", "typescript-publish-state.sh"));
  });
  try {
    expect(current("false\n")()).toBe("go,rust");
    expect(current("true\n")()).toBe("go,rust,typescript");
    expect(current("enabled\n")).toThrow();
    expect(target(() => {})()).toBe("go");
    expect(target(root => writeFileSync(
      join(root, ".github", "workflows", "release-rust.yml"), "name: Rust\n",
    ))()).toBe("go,rust");
    expect(target(() => {}, "push")).toThrow();
    expect(target(root => writeFileSync(
      join(root, ".github", "typescript-publish-enabled"), "false\n",
    ))).toThrow();
    expect(target(root => copyFileSync(
      stateHelper, join(root, "scripts", "typescript-publish-state.sh"),
    ))).toThrow();
    expect(target(root => {
      writeFileSync(join(root, ".github", "typescript-publish-enabled"), "false\n");
      writeFileSync(join(root, "scripts", "typescript-publish-state.sh"), "exit 7\n");
    })).toThrow();
    expect(target(root => {
      writeFileSync(join(root, ".github", "typescript-publish-enabled"), "false\n");
      writeFileSync(join(root, "scripts", "typescript-publish-state.sh"), "echo maybe\n");
    })).toThrow();
  } finally {
    rmSync(temp, { recursive: true, force: true });
  }
});

it("rejects malformed publication state before release tags or pushes", () => {
  const temp = mkdtempSync(join(tmpdir(), "hey-release-preflight-"));
  const version = "1.2.3";
  try {
    for (const directory of [
      ".github",
      "scripts",
      join("go", "pkg", "hey"),
      join("rust", "hey-sdk"),
      "bin",
    ])
      mkdirSync(join(temp, directory), { recursive: true });
    copyFileSync(new URL("../../Makefile", import.meta.url), join(temp, "Makefile"));
    copyFileSync(
      new URL("../../scripts/typescript-publish-state.sh", import.meta.url),
      join(temp, "scripts", "typescript-publish-state.sh"),
    );
    writeFileSync(
      join(temp, "go", "pkg", "hey", "version.go"),
      `package hey\n\nconst Version = "${version}"\n`,
    );
    writeFileSync(
      join(temp, "rust", "hey-sdk", "Cargo.toml"),
      `[package]\nversion = "${version}"\n`,
    );
    const calls = join(temp, "git-calls");
    const git = join(temp, "bin", "git");
    const node = join(temp, "bin", "node");
    writeFileSync(git, '#!/bin/sh\nprintf \'%s\\n\' "$*" >> "$MOCK_CALLS"\n');
    writeFileSync(node, "#!/bin/sh\nexit 0\n");
    chmodSync(git, 0o755);
    chmodSync(node, 0o755);
    const env = {
      ...process.env,
      PATH: `${join(temp, "bin")}${delimiter}${process.env.PATH ?? ""}`,
      MOCK_CALLS: calls,
    };
    const release = (state: string) => {
      writeFileSync(join(temp, ".github", "typescript-publish-enabled"), state);
      writeFileSync(calls, "");
      let error: unknown;
      try {
        execFileSync(
          "make",
          ["release", `VERSION=${version}`, "MAKE=/bin/true"],
          { cwd: temp, env, encoding: "utf8", stdio: "pipe" },
        );
      } catch (cause) {
        error = cause;
      }
      return { error, calls: readFileSync(calls, "utf8") };
    };

    for (const state of [" false\n", "false\r\n"]) {
      const result = release(state);
      expect(result.error).toBeDefined();
      expect(result.calls).not.toMatch(/(^|\n)(tag|push) /);
    }

    const valid = release("false\n");
    expect(valid.error).toBeUndefined();
    expect(valid.calls).toContain(`tag v${version}`);
    expect(valid.calls).toContain(`tag go/v${version}`);
    expect(valid.calls).toContain(
      `push origin v${version} go/v${version}`,
    );
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
