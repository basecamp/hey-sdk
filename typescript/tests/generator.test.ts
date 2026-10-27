import { readFileSync } from "node:fs";
import { it, expect } from "vitest";
import {
  generateOperations,
  render,
  type Spec,
  type Behavior,
} from "../scripts/generate.js";
import {
  GeneratedOperations,
  operationMetadata,
} from "../src/generated/operations.js";
import {
  HeyClient,
  isPostingBundle,
  isRecordingCalendarTimeTrack,
  isRecordingCalendarTodo,
  type components,
} from "../src/index.js";
const spec = JSON.parse(
  readFileSync(new URL("../../openapi.json", import.meta.url), "utf8"),
) as Spec;
const behavior = JSON.parse(
  readFileSync(new URL("../../behavior-model.json", import.meta.url), "utf8"),
) as Behavior;
it("generates exactly all modeled operations, without account stripping or unsafe PUT retry inference", () => {
  const output = generateOperations(spec, behavior);
  const expected = Object.values(spec.paths)
    .flatMap((item) =>
      Object.values(item)
        .filter((o) => o.operationId)
        .map((o) => o.operationId),
    )
    .sort();
  expect(JSON.parse(output["coverage.json"]).operations).toEqual(expected);
  expect(Object.keys(operationMetadata).sort()).toEqual(expected);
  expect(
    Object.getOwnPropertyNames(GeneratedOperations.prototype)
      .filter((n) => n !== "constructor")
      .sort(),
  ).toEqual(expected.map((n) => n[0]!.toLowerCase() + n.slice(1)).sort());
  expect(operationMetadata.DeleteExtenzion.path).toContain("{accountId}");
  expect(operationMetadata.UpdateMessage.safe).toBe(false);
  expect(operationMetadata.GetOngoingTimeTrack.emptyOn).toEqual([404]);
});
it("fails on missing behavior, duplicate operations and unsupported request media", () => {
  const b = structuredClone(behavior);
  delete b.operations.ListBoxes;
  expect(() => generateOperations(spec, b)).toThrow();
  const s = structuredClone(spec);
  s.paths["/duplicate"] = {
    get: structuredClone(s.paths["/boxes.json"]!.get!),
  };
  expect(() => generateOperations(s, behavior)).toThrow("duplicate");
  const m = structuredClone(spec);
  m.paths["/messages.json"]!.post!.requestBody!.content = {
    "text/plain": { schema: { type: "string" } },
  };
  expect(() => generateOperations(m, behavior)).toThrow("media");
});
it("models Recording.category as narrowly nullable at runtime and compile time", async () => {
  const category: components["schemas"]["Recording"]["category"] = null;
  expect(category).toBeNull();
  const response = await new HeyClient({
    token: "test",
    fetch: async () =>
      Response.json({
        time_tracks: [
          { id: 1, type: "Calendar::TimeTrack", category: null },
        ],
        categories: [],
      }),
  }).listTimeTracks();
  expect(response.data?.time_tracks?.[0]?.category).toBeNull();
  const requestCategory: components["schemas"]["UpdateTimeTrackPayload"]["category_title"] = "work";
  expect(requestCategory).toBe("work");
  // @ts-expect-error Request category titles are strings, not nullable response fields.
  const invalidRequestCategory: components["schemas"]["UpdateTimeTrackPayload"]["category_title"] = null;
  expect(invalidRequestCategory).toBeNull();
});
it("preserves polymorphic optional fields while narrowing discriminators", () => {
  expect(isPostingBundle({ id: 1, kind: "bundle" })).toBe(true);
  expect(isPostingBundle({ id: 1, kind: "topic" })).toBe(false);
  expect(isRecordingCalendarTodo({ id: 1, type: "CalendarTodo" })).toBe(true);
  expect(isRecordingCalendarTodo({ id: 1, type: "Calendar::Todo" })).toBe(true);
  expect(
    isRecordingCalendarTimeTrack({ id: 1, type: "Calendar::TimeTrack" }),
  ).toBe(true);
  expect(isRecordingCalendarTimeTrack({ id: 1, type: "Calendar::Todo" })).toBe(
    false,
  );
});
it("declares the account filter on every signed operation and none on an unsigned one", () => {
  // Generic OpenAPI clients learn their inputs from standard parameters only. The SDK
  // transports already send filtered_account_id, so this is the one place it is declared.
  const s = spec as unknown as {
    "x-hey-account-scope"?: { query?: string };
    security?: unknown;
    paths: Record<string, Record<string, Record<string, any>>>;
  };
  expect(s["x-hey-account-scope"]).toEqual({ query: "filtered_account_id" });
  const filter = {
    name: "filtered_account_id",
    in: "query",
    description:
      "Optional account filter, an id from GetIdentity's accounts[].id. Reads that filter by account narrow to that account. Omit it to leave the filter unset, and what an unfiltered request returns depends on the operation. Some operations ignore it. To choose a sending address, use an operation's explicit sender-selection fields rather than relying on this filter. GetIdentity also lists accounts the filter does not accept, and an id HEY does not recognize or cannot use for this identity can leave the filter unset instead of returning an error.",
    required: false,
    schema: { type: "integer", format: "int64" },
    "x-hey-account-scope": true,
  };
  const seen = { signed: 0, unsigned: 0 };
  for (const [path, item] of Object.entries(s.paths)) {
    expect(item.parameters, path).toBeUndefined();
    for (const o of Object.values(item)) {
      const id = o.operationId as string | undefined;
      if (!id) continue;
      const unsigned = o["x-hey-unsigned"] !== undefined;
      const effective = "security" in o ? o.security : s.security;
      expect((effective as unknown[]).length === 0, id).toBe(unsigned);
      const named = ((o.parameters ?? []) as { name: string }[]).filter(
        (p) => p.name === "filtered_account_id",
      );
      expect(named, id).toEqual(unsigned ? [] : [filter]);
      seen[unsigned ? "unsigned" : "signed"]++;
    }
  }
  expect(seen.signed).toBeGreaterThan(0);
  expect(seen.unsigned).toBeGreaterThan(0);
});
it("leaves the marked account filter out of schema.ts and operations.ts, but not an unmarked one", async () => {
  const filter = (marked: boolean) => ({
    name: "filtered_account_id",
    in: "query",
    schema: { type: "integer", format: "int64" },
    ...(marked ? { "x-hey-account-scope": true } : {}),
  });
  const response = { "200": { description: "ok" } };
  const fixture = {
    openapi: "3.1.0",
    info: { title: "fixture", version: "1" },
    paths: {
      "/boxes.json": {
        get: {
          operationId: "ListBoxes",
          parameters: [
            { name: "page", in: "query", schema: { type: "string" } },
            filter(true),
          ],
          responses: response,
        },
      },
      "/identity.json": {
        get: { operationId: "GetIdentity", parameters: [filter(true)], responses: response },
      },
      "/legacy.json": {
        get: { operationId: "ListLegacy", parameters: [filter(false)], responses: response },
      },
    },
    components: { schemas: {} },
  } as unknown as Spec;
  const before = structuredClone(fixture);
  const files = await render(fixture, {
    operations: {
      ListBoxes: { readonly: true, idempotent: false },
      GetIdentity: { readonly: true, idempotent: false },
      ListLegacy: { readonly: true, idempotent: false },
    },
  });
  expect(fixture).toEqual(before);
  const operation = (source: string, id: string) =>
    source.slice(source.indexOf(`${id}: {`)).split(/\n {4}\w+: \{/)[0]!;
  const schema = files["schema.ts"]!;
  expect(operation(schema, "ListBoxes")).toContain("page?: string");
  expect(operation(schema, "ListBoxes")).not.toContain("filtered_account_id");
  expect(operation(schema, "GetIdentity")).not.toContain("filtered_account_id");
  expect(operation(schema, "ListLegacy")).toContain("filtered_account_id?:");
  const metadata = JSON.parse(
    files["operations.ts"]!
      .split("export const operationMetadata = ")[1]!
      .split(" as const;")[0]!,
  ) as Record<string, { parameters: { name: string }[] }>;
  expect(metadata.ListBoxes!.parameters.map((p) => p.name)).toEqual(["page"]);
  expect(metadata.GetIdentity!.parameters).toEqual([]);
  expect(metadata.ListLegacy!.parameters.map((p) => p.name)).toEqual([
    "filtered_account_id",
  ]);
});
it("refuses the account filter as an operation input, leaving it to the client's account scope", async () => {
  let sent = false;
  const c = new HeyClient({
    token: "test",
    fetch: async () => {
      sent = true;
      return new Response(null, { status: 204 });
    },
  });
  await expect(
    c.listBoxes({ query: { filtered_account_id: 17 } } as never),
  ).rejects.toMatchObject({ name: "HeyError", code: "usage" });
  expect(sent).toBe(false);
});
// Independent OpenAPI-driven request coverage. Every generated method must reach the
// real transport with the modeled method/path/query/body; new operations join automatically.
for (const [path, item] of Object.entries(spec.paths))
  for (const [method, op] of Object.entries(item)) {
    if (!op.operationId) continue;
    it(`generated ${op.operationId} sends modeled ${method.toUpperCase()} ${path}`, async () => {
      let request: Request | undefined;
      const c = new HeyClient({
        token: "test",
        fetch: async (input, init) => {
          request = new Request(input, init);
          return new Response(null, { status: 204 });
        },
      });
      const input: Record<string, any> = { path: {}, query: {} };
      let expectedPath = path;
      for (const p of op.parameters ?? []) {
        if ("x-hey-account-scope" in p) continue;
        const value =
          p.schema.type === "integer"
            ? 17
            : p.schema.type === "boolean"
              ? true
              : "opaque-value";
        input[p.in]![p.name] = value;
        if (p.in === "path")
          expectedPath = expectedPath.replace(`{${p.name}}`, String(value));
      }
      if (op.requestBody) input.body = {};
      const name = op.operationId[0]!.toLowerCase() + op.operationId.slice(1);
      await (c as any)[name](input);
      expect(request).toBeDefined();
      expect(request!.method).toBe(method.toUpperCase());
      expect(new URL(request!.url).pathname).toBe(expectedPath);
      for (const [key, value] of Object.entries(input.query))
        expect(new URL(request!.url).searchParams.get(key)).toBe(String(value));
      expect(await request!.text()).toBe(op.requestBody ? "{}" : "");
    });
  }
it("reads x-hey-unsigned and x-hey-no-redirect into route metadata, not the operation's name", () => {
  const meta = (output: ReturnType<typeof generateOperations>) =>
    JSON.parse(
      output["operations.ts"]
        .split("export const operationMetadata = ")[1]!
        .split(" as const;")[0]!,
    ) as Record<string, { unsigned?: boolean; noRedirect?: boolean; safe: boolean }>;
  const generated = meta(generateOperations(spec, behavior));
  for (const id of ["CreateToken", "RevokeToken"]) {
    expect(generated[id]!.unsigned).toBe(true);
    expect(generated[id]!.noRedirect).toBe(true);
  }
  for (const [id, m] of Object.entries(generated))
    if (!["CreateToken", "RevokeToken"].includes(id)) {
      expect(m.unsigned, id).toBeUndefined();
      expect(m.noRedirect, id).toBeUndefined();
    }
  const s = structuredClone(spec);
  s.paths["/boxes.json"]!.get!["x-hey-unsigned"] = {};
  s.paths["/boxes.json"]!.get!["x-hey-no-redirect"] = {};
  delete s.paths["/oauth/tokens.json"]!.post!["x-hey-unsigned"];
  delete s.paths["/oauth/tokens.json"]!.post!["x-hey-no-redirect"];
  const moved = meta(generateOperations(s, behavior));
  expect(moved.ListBoxes).toMatchObject({ unsigned: true, noRedirect: true });
  expect(moved.CreateToken!.unsigned).toBeUndefined();
  expect(moved.CreateToken!.noRedirect).toBeUndefined();
});
it("lets the verb stand in for idempotency when the behavior model says neither", () => {
  // RevokeToken is a DELETE the model calls neither readonly nor idempotent; its declared
  // retry policy resends a 429 or 503 only if the verb stands in, as every other SDK reads it.
  expect(operationMetadata.RevokeToken.safe).toBe(true);
  expect(operationMetadata.CreateToken.safe).toBe(false);
  expect(operationMetadata.UpdateMessage.safe).toBe(false);
});
it("carries each operation's unsigned and no-redirect flags into the behavior model", () => {
  // A consumer that reads only behavior-model.json (the MCP toolkit, an agent's policy layer)
  // learns there which operations go without credentials and refuse a redirect.
  for (const item of Object.values(spec.paths))
    for (const o of Object.values(item) as Record<string, unknown>[]) {
      const id = o.operationId as string | undefined;
      if (!id) continue;
      const b = behavior.operations[id] as { unsigned?: boolean; no_redirect?: boolean };
      expect(b.unsigned, id).toBe(o["x-hey-unsigned"] !== undefined);
      expect(b.no_redirect, id).toBe(o["x-hey-no-redirect"] !== undefined);
    }
  expect(behavior.operations.CreateToken).toMatchObject({ unsigned: true, no_redirect: true });
  expect(behavior.operations.RevokeToken).toMatchObject({ unsigned: true, no_redirect: true });
});
it("declares bearer auth on every signed operation and none on an unsigned one", () => {
  // Generic OpenAPI tooling reads credentials from the standard security fields, not from
  // x-hey-unsigned, so the two must say the same thing about every operation.
  const s = spec as unknown as {
    security?: unknown;
    components: { securitySchemes?: Record<string, { type?: string; scheme?: string }> };
  };
  const schemes = s.components.securitySchemes ?? {};
  expect(Object.values(schemes).map((x) => [x.type, x.scheme?.toLowerCase()])).toEqual([
    ["http", "bearer"],
  ]);
  const bearer = [{ [Object.keys(schemes)[0]!]: [] }];
  expect(s.security).toEqual(bearer);
  for (const item of Object.values(spec.paths))
    for (const o of Object.values(item) as Record<string, unknown>[]) {
      const id = o.operationId as string | undefined;
      if (!id) continue;
      const effective = "security" in o ? o.security : s.security;
      expect(effective, id).toEqual(o["x-hey-unsigned"] !== undefined ? [] : bearer);
    }
});
it("marks the operations whose refusals are RFC 6749's", () => {
  for (const [id, m] of Object.entries(operationMetadata))
    expect((m as { oauthErrors?: true }).oauthErrors, id).toBe(
      ["CreateToken", "RevokeToken"].includes(id) ? true : undefined,
    );
});
