import { readFileSync } from "node:fs";
import { it, expect } from "vitest";
import {
  generateOperations,
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
