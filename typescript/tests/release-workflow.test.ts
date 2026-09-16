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
import { delimiter, dirname, join } from "node:path";
import { execFile, execFileSync } from "node:child_process";
import { createServer } from "node:http";
import type { AddressInfo } from "node:net";
import { promisify } from "node:util";
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

  // Kotlin is switched the same way, from the tagged commit, and is off until GitHub
  // Packages publishing is sorted out.
  const kotlinState = readFileSync(
    new URL("../../.github/kotlin-publish-enabled", import.meta.url),
    "utf8",
  ).trim();
  const kotlinWorkflow = readFileSync(
    new URL("../../.github/workflows/release-kotlin.yml", import.meta.url),
    "utf8",
  );
  expect(kotlinState).toBe("false");
  expect(kotlinWorkflow).toContain("enabled=$(bash scripts/kotlin-publish-state.sh)");
  expect(kotlinWorkflow).toContain(
    "if: github.event_name == 'push' && needs.test.outputs.publish_enabled == 'true'",
  );
  expect(kotlinWorkflow).toContain(
    "if: github.event_name == 'workflow_dispatch' || needs.test.outputs.publish_enabled != 'true'",
  );
});

it("selects current and historical release languages without weakening state validation", () => {
  const temp = mkdtempSync(join(tmpdir(), "hey-release-languages-"));
  const selector = new URL("../../scripts/release-languages.sh", import.meta.url).pathname;
  const stateHelper = new URL("../../scripts/typescript-publish-state.sh", import.meta.url).pathname;
  const kotlinHelper = new URL("../../scripts/kotlin-publish-state.sh", import.meta.url).pathname;
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
  const withKotlin = (typescript: string, kotlin: string) => target(root => {
    writeFileSync(join(root, ".github", "typescript-publish-enabled"), typescript);
    copyFileSync(stateHelper, join(root, "scripts", "typescript-publish-state.sh"));
    writeFileSync(join(root, ".github", "kotlin-publish-enabled"), kotlin);
    copyFileSync(kotlinHelper, join(root, "scripts", "kotlin-publish-state.sh"));
  });
  try {
    expect(current("false\n")()).toBe("go,rust");
    expect(current("true\n")()).toBe("go,rust,typescript");
    expect(current("enabled\n")).toThrow();
    expect(withKotlin("false\n", "false\n")()).toBe("go,rust");
    expect(withKotlin("false\n", "true\n")()).toBe("go,rust,kotlin");
    expect(withKotlin("true\n", "true\n")()).toBe("go,rust,typescript,kotlin");
    expect(withKotlin("false\n", "enabled\n")).toThrow();
    // Swift has no switch: a target with its release workflow waits for it, whatever else is on.
    const withSwift = (setup: (root: string) => void) => target(root => {
      setup(root);
      writeFileSync(join(root, ".github", "workflows", "release-swift.yml"), "name: Swift\n");
    });
    expect(withSwift(root => {
      writeFileSync(join(root, ".github", "typescript-publish-enabled"), "false\n");
      copyFileSync(stateHelper, join(root, "scripts", "typescript-publish-state.sh"));
    })()).toBe("go,rust,swift");
    expect(withSwift(root => {
      writeFileSync(join(root, ".github", "typescript-publish-enabled"), "true\n");
      copyFileSync(stateHelper, join(root, "scripts", "typescript-publish-state.sh"));
      writeFileSync(join(root, ".github", "kotlin-publish-enabled"), "true\n");
      copyFileSync(kotlinHelper, join(root, "scripts", "kotlin-publish-state.sh"));
    })()).toBe("go,rust,typescript,kotlin,swift");
    expect(withSwift(root => {
      writeFileSync(join(root, ".github", "typescript-publish-enabled"), "enabled\n");
      copyFileSync(stateHelper, join(root, "scripts", "typescript-publish-state.sh"));
    })).toThrow();
    expect(target(root => {
      writeFileSync(join(root, ".github", "typescript-publish-enabled"), "false\n");
      copyFileSync(stateHelper, join(root, "scripts", "typescript-publish-state.sh"));
      writeFileSync(join(root, ".github", "kotlin-publish-enabled"), "true\n");
    })).toThrow();
    expect(target(root => {
      writeFileSync(join(root, ".github", "typescript-publish-enabled"), "false\n");
      copyFileSync(stateHelper, join(root, "scripts", "typescript-publish-state.sh"));
      writeFileSync(join(root, ".github", "workflows", "release-kotlin.yml"), "name: Kotlin\n");
    })()).toBe("go,rust");
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
      join("kotlin", "sdk", "src", "commonMain", "kotlin", "com", "basecamp", "hey"),
      join("swift", "Sources", "Hey"),
      "bin",
    ])
      mkdirSync(join(temp, directory), { recursive: true });
    copyFileSync(new URL("../../Makefile", import.meta.url), join(temp, "Makefile"));
    copyFileSync(
      new URL("../../scripts/typescript-publish-state.sh", import.meta.url),
      join(temp, "scripts", "typescript-publish-state.sh"),
    );
    copyFileSync(
      new URL("../../scripts/kotlin-publish-state.sh", import.meta.url),
      join(temp, "scripts", "kotlin-publish-state.sh"),
    );
    writeFileSync(
      join(temp, "go", "pkg", "hey", "version.go"),
      `package hey\n\nconst Version = "${version}"\n`,
    );
    writeFileSync(
      join(temp, "rust", "hey-sdk", "Cargo.toml"),
      `[package]\nversion = "${version}"\n`,
    );
    writeFileSync(
      join(temp, "kotlin", "sdk", "build.gradle.kts"),
      `version = "${version}"\n`,
    );
    writeFileSync(
      join(temp, "kotlin", "sdk", "src", "commonMain", "kotlin", "com", "basecamp", "hey", "HeyConfig.kt"),
      `        const val VERSION = "${version}"\n`,
    );
    const swiftConfig = join(temp, "swift", "Sources", "Hey", "HeyConfig.swift");
    writeFileSync(swiftConfig, `    public static let version = "${version}"\n`);
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
    const release = (state: string, kotlin = "false\n") => {
      writeFileSync(join(temp, ".github", "typescript-publish-enabled"), state);
      writeFileSync(join(temp, ".github", "kotlin-publish-enabled"), kotlin);
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
    const kotlinMalformed = release("false\n", "enabled\n");
    expect(kotlinMalformed.error).toBeDefined();
    expect(kotlinMalformed.calls).not.toMatch(/(^|\n)(tag|push) /);

    writeFileSync(swiftConfig, `    public static let version = "0.0.1"\n`);
    const swiftStale = release("false\n");
    expect(swiftStale.error).toBeDefined();
    expect(swiftStale.calls).not.toMatch(/(^|\n)(tag|push) /);
    writeFileSync(swiftConfig, `    public static let version = "${version}"\n`);

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

it("publishes the Kotlin library only to an empty version and skips only an identical one", async () => {
  const workflow = readFileSync(
    new URL("../../.github/workflows/release-kotlin.yml", import.meta.url),
    "utf8",
  );
  expect(workflow).toContain("scripts/kotlin-packages-state.sh sdk/build/staging-repo");
  expect(workflow).toContain("if: steps.check.outputs.state == 'absent'");
  expect(workflow).not.toContain("409");

  const temp = mkdtempSync(join(tmpdir(), "hey-kotlin-release-"));
  const script = new URL("../../scripts/kotlin-packages-state.sh", import.meta.url).pathname;
  const staging = join(temp, "staging");
  const files: Record<string, string> = {
    "com/basecamp/hey-sdk/1.2.3/hey-sdk-1.2.3.pom": "<project/>",
    "com/basecamp/hey-sdk/1.2.3/hey-sdk-1.2.3.module": "{}",
    "com/basecamp/hey-sdk-jvm/1.2.3/hey-sdk-jvm-1.2.3.jar": "jar bytes",
    "com/basecamp/hey-sdk-jvm/1.2.3/hey-sdk-jvm-1.2.3.pom": "<project/>",
  };
  for (const [path, content] of Object.entries(files)) {
    mkdirSync(dirname(join(staging, path)), { recursive: true });
    writeFileSync(join(staging, path), content);
    writeFileSync(join(staging, `${path}.sha1`), "sidecar");
  }
  writeFileSync(join(staging, "com/basecamp/hey-sdk/maven-metadata.xml"), "<metadata/>");

  const remote = new Map<string, string>();
  let broken = false;
  const seen: string[] = [];
  const authorizations = new Set<string | undefined>();
  const server = createServer((request, response) => {
    seen.push(request.url ?? "");
    authorizations.add(request.headers.authorization);
    if (broken) {
      response.writeHead(500);
      response.end();
      return;
    }
    const body = remote.get((request.url ?? "").slice(1));
    if (body === undefined) {
      response.writeHead(404);
      response.end("not here");
      return;
    }
    response.writeHead(200);
    response.end(body);
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address() as AddressInfo;
  const run = async () => {
    const { stdout } = await promisify(execFile)(
      "bash",
      [script, staging, `http://127.0.0.1:${port}/`],
      {
        encoding: "utf8",
        env: { ...process.env, GITHUB_USER: "x-access-token", GITHUB_ACCESS_TOKEN: "token" },
      },
    );
    return stdout.trim();
  };
  try {
    expect(await run()).toBe("absent");
    expect(seen.sort()).toEqual(Object.keys(files).map((path) => `/${path}`).sort());
    expect([...authorizations]).toEqual([
      `Basic ${Buffer.from("x-access-token:token").toString("base64")}`,
    ]);

    for (const [path, content] of Object.entries(files)) remote.set(path, content);
    expect(await run()).toBe("published");

    const jar = "com/basecamp/hey-sdk-jvm/1.2.3/hey-sdk-jvm-1.2.3.jar";
    remote.delete(jar);
    await expect(run()).rejects.toThrow(/missing\s+com\/basecamp\/hey-sdk-jvm\/1\.2\.3\/hey-sdk-jvm-1\.2\.3\.jar/);
    await expect(run()).rejects.toThrow(/Delete the version/);

    remote.set(jar, "other bytes");
    await expect(run()).rejects.toThrow(/different\s+com\/basecamp\/hey-sdk-jvm/);

    broken = true;
    await expect(run()).rejects.toThrow(/answered 500/);

    const empty = join(temp, "empty");
    mkdirSync(empty);
    await expect(
      promisify(execFile)("bash", [script, empty, `http://127.0.0.1:${port}/`], {
        env: { ...process.env, GITHUB_USER: "x-access-token", GITHUB_ACCESS_TOKEN: "token" },
      }),
    ).rejects.toThrow(/nothing staged/);
  } finally {
    server.close();
    rmSync(temp, { recursive: true, force: true });
  }
});
