// Fixture argument adapters mirror the Go harness, not SDK API implementations.
// All network behavior, retries, parsing, security and assertions use the real SDK.
import { repetitionCount, validateFixture } from "./contract.js";
import { assertBodyFields } from "./assertions.js";
import { readFile, readdir } from "node:fs/promises";
import assert from "node:assert/strict";
import { createServer, type IncomingMessage } from "node:http";
import {
  HeyClient,
  HeyError,
  type OperationName,
} from "../../../typescript/src/index.js";
import { operationMetadata } from "../../../typescript/src/generated/operations.js";
import { parseJSON, stringifyJSON } from "../../../typescript/src/security.js";

type RecordValue = Record<string, any>;
interface Fixture extends RecordValue {
  name: string;
  operation: string;
  mockResponses: RecordValue[];
  assertions: (RecordValue & { type: string })[];
}
const testsDir = new URL("../../tests/", import.meta.url);
const spec = JSON.parse(
  await readFile(new URL("../../../openapi.json", import.meta.url), "utf8"),
);
const exclusions: {
  file: string;
  name: string;
  operation: string;
  reason: string;
}[] = JSON.parse(
  await readFile(new URL("./not-applicable.json", import.meta.url), "utf8"),
);
const usedExclusions = new Set<string>();
const inventory: Record<string, { name: string; operation: string }[]> =
  JSON.parse(
    await readFile(
      new URL("./fixture-inventory.json", import.meta.url),
      "utf8",
    ),
  );
assert.equal(
  exclusions.length,
  3,
  "Applicability boundary changed; review required",
);
const schemas = spec.components.schemas;
const operations = Object.fromEntries(
  Object.values(spec.paths).flatMap((p: any) =>
    Object.values(p)
      .filter((o: any) => o.operationId)
      .map((o: any) => [o.operationId, o]),
  ),
);
const aliases: Record<string, OperationName> = {
  CreateDraft: "CreateMessage",
  UpdateDraft: "UpdateMessage",
  SendDraft: "UpdateMessage",
  CreateReplyDraft: "CreateReply",
};
function resolve(s: any): any {
  return s?.$ref ? schemas[s.$ref.split("/").at(-1)] : s;
}
function bodyFor(s: any, values: RecordValue): RecordValue {
  s = resolve(s);
  const body: RecordValue = {};
  for (const [name, field] of Object.entries(s?.properties ?? {}) as [
    string,
    any,
  ][]) {
    if (Object.hasOwn(values, name)) body[name] = values[name];
    else if (resolve(field)?.type === "object")
      body[name] = bodyFor(field, values);
    else if (s.required?.includes(name)) {
      if (field.type === "string") body[name] = "";
      else if (field.type === "integer") body[name] = 0;
      else if (field.type === "array") body[name] = [];
    }
  }
  return body;
}
function adapt(tc: Fixture): { operation: OperationName; input: RecordValue } {
  const operation = aliases[tc.operation] ?? (tc.operation as OperationName);
  if (!Object.hasOwn(operationMetadata, operation))
    throw new Error(`Unknown operation: ${tc.operation}`);
  const op = operations[operation];
  const values = { ...tc.requestBody };
  if (operation === "UpdateJournalEntry") values.content = values.body;
  const input: RecordValue = {
    path: { ...tc.pathParams },
    query: { ...tc.queryParams },
  };
  if (op.requestBody)
    input.body = bodyFor(
      op.requestBody.content["application/json"].schema,
      values,
    );
  if (["CreateMessage", "UpdateMessage", "CreateReply"].includes(operation)) {
    input.body = {
      message: { subject: values.subject ?? "", content: values.content ?? "" },
    };
    if (values.acting_sender_id !== undefined || operation === "CreateReply")
      input.body.acting_sender_id = values.acting_sender_id ?? 0;
    if (tc.operation.includes("Draft") && tc.operation !== "SendDraft")
      input.body.entry = { status: "drafted" };
    if (values.to)
      input.body.entry = {
        ...input.body.entry,
        addressed: { directly: values.to },
      };
    if (operation === "UpdateMessage" && input.path.entryId !== undefined)
      input.path.messageId = input.path.entryId;
  }
  if (
    operation === "DeleteCalendarEventOccurrence" &&
    input.path.occurrenceId
  ) {
    const [eventId, occurrence] = String(input.path.occurrenceId).split("_");
    input.path = { eventId: Number(eventId), occurrence };
    input.query.apply_to_future = values.scope === "this_and_following";
  }
  return { operation, input };
}
function at(value: any, path: string): any {
  return path.split(".").reduce((v, k) => v?.[k], value);
}
function queryFields(query: URLSearchParams, expected: RecordValue) {
  for (const [key, value] of Object.entries(expected)) {
    assert.equal(query.get(key), value === null ? null : String(value));
    if (value !== null) assert.equal(query.getAll(key).length, 1);
  }
}
async function run(tc: Fixture) {
  const { operation, input } = adapt(tc);
  const requests: {
    url: URL;
    headers: IncomingMessage["headers"];
    method: string;
    body: string;
    time: number;
  }[] = [];
  let serverFailure: Error | undefined;
  const server = createServer(async (req, res) => {
    try {
      const chunks = [];
      for await (const chunk of req) chunks.push(chunk);
      const index = requests.length;
      requests.push({
        url: new URL(req.url!, "http://localhost"),
        headers: req.headers,
        method: req.method!,
        body: Buffer.concat(chunks).toString(),
        time: Date.now(),
      });
      const mock = tc.mockResponses[index];
      if (!mock) throw new Error(`Unexpected request ${index + 1}`);
      if (mock.delay) await new Promise((r) => setTimeout(r, mock.delay));
      res.writeHead(mock.status, {
        "Content-Type": "application/json",
        ...mock.headers,
      });
      res.end(mock.body === undefined ? undefined : stringifyJSON(mock.body));
    } catch (error) {
      serverFailure = error as Error;
      res.writeHead(500);
      res.end();
    }
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = (server.address() as { port: number }).port;
  const baseUrl = `http://127.0.0.1:${port}`;
  let error: any, result: any;
  try {
    let token = "conformance-test-token";
    let client = new HeyClient({
      baseUrl: tc.configOverrides?.baseUrl ?? baseUrl,
      token: tc.configOverrides?.refreshableCredentials
        ? {
            getToken: () => token,
            refresh: () => {
              token = "conformance-refreshed-token";
            },
          }
        : token,
      cache: !!tc.configOverrides?.cacheEnabled,
    });
    if (tc.configOverrides?.accountId)
      client = await client.forAccount(tc.configOverrides.accountId);
    const method = operation[0]!.toLowerCase() + operation.slice(1);
    for (
      let repeat = 0;
      repeat < repetitionCount(tc.repeatOperation ?? 0);
      repeat++
    ) {
      result = await (client as any)[method](
        input,
        tc.configOverrides?.clientLayer === "hey" ? { format: "json" } : {},
      );
    }
  } catch (e) {
    error = e;
  } finally {
    await new Promise<void>((resolve, reject) =>
      server.close((e) => (e ? reject(e) : resolve())),
    );
  }
  if (serverFailure) throw serverFailure;
  const first = requests[0];
  const last = requests.at(-1);
  for (const a of tc.assertions) {
    switch (a.type) {
      case "requestCount":
        assert.equal(requests.length, a.expected);
        break;
      case "delayBetweenRequests":
        assert.ok(requests.length > 1);
        for (let i = 1; i < requests.length; i++) {
          const ms = requests[i]!.time - requests[i - 1]!.time;
          if (a.min !== undefined)
            assert.ok(ms >= a.min, `delay ${ms} < ${a.min}`);
          if (a.max !== undefined) assert.ok(ms <= a.max);
        }
        break;
      case "noError":
        assert.equal(error, undefined);
        break;
      case "errorCode":
        assert.ok(error instanceof HeyError, String(error));
        assert.equal(error.code, a.expected);
        break;
      case "errorField":
        assert.ok(error);
        assert.ok(
          ["httpStatus", "retryable", "requestId"].includes(a.path),
          `Unknown error field ${a.path}`,
        );
        assert.deepEqual(at(error, a.path), a.expected);
        break;
      case "statusCode":
        assert.equal(result?.status ?? error?.httpStatus, a.expected);
        break;
      case "requestPath":
        assert.equal(first?.url.pathname, a.expected);
        break;
      case "lastRequestPath":
        assert.equal(last?.url.pathname, a.expected);
        break;
      case "requestMethod":
        assert.equal(first?.method, a.expected);
        break;
      case "requestQuery":
        assert.ok(first);
        queryFields(first.url.searchParams, a.expected);
        break;
      case "lastRequestQuery":
        assert.ok(last);
        queryFields(last.url.searchParams, a.expected);
        break;
      case "requestBody":
        assert.ok(first);
        assertBodyFields(parseJSON(first.body), a.expected);
        break;
      case "requestForm":
      case "lastRequestForm": {
        const req = a.type === "requestForm" ? first : last;
        assert.ok(req);
        queryFields(new URLSearchParams(req.body), a.expected);
        break;
      }
      case "headerPresent":
        assert.ok(first?.headers[a.path.toLowerCase()]);
        break;
      case "lastRequestHeader":
        assert.equal(last?.headers[a.path.toLowerCase()] ?? "", a.expected);
        break;
      case "responseMeta":
        assert.ok(
          ["totalCount", "nextPage"].includes(a.path),
          `Unknown response metadata ${a.path}`,
        );
        assert.deepEqual(at(result, a.path), a.expected);
        break;
      case "urlOrigin":
        assert.equal(a.expected, "rejected");
        assert.ok(
          error instanceof HeyError &&
            error.code === "usage" &&
            /origin/.test(error.message),
          String(error),
        );
        break;
      case "responseBody":
        assert.deepEqual(at(result?.data, a.path), a.expected);
        break;
      default:
        throw new Error(`Unknown assertion ${a.type}`);
    }
  }
}
let passed = 0,
  failed = 0,
  excluded = 0,
  total = 0;
const files = (await readdir(testsDir))
  .filter((f) => f.endsWith(".json"))
  .sort();
assert.ok(files.length, "No fixtures found");
assert.deepEqual(
  files,
  Object.keys(inventory).sort(),
  "Fixture files changed without inventory update",
);
for (const file of files) {
  const fixtures = parseJSON(
    await readFile(new URL(file, testsDir), "utf8"),
  ) as Fixture[];
  assert.ok(
    Array.isArray(fixtures) && fixtures.length,
    `Empty/invalid fixture file ${file}`,
  );
  assert.deepEqual(
    fixtures.map(({ name, operation }) => ({ name, operation })),
    inventory[file],
    `Fixture identities changed: ${file}`,
  );
  const names = new Set<string>();
  for (const tc of fixtures) {
    total++;
    validateFixture(tc);
    assert.ok(!names.has(tc.name), `Duplicate fixture ${file}: ${tc.name}`);
    names.add(tc.name);
    const exclusion = exclusions.find(
      (e) => e.file === file && e.name === tc.name,
    );
    if (exclusion) {
      assert.equal(tc.operation, exclusion.operation);
      assert.equal(tc.operation, "UpdateCalendarEvent");
      assert.ok(
        !operations[tc.operation],
        "Modeled operation cannot be excluded",
      );
      assert.ok(exclusion.reason.trim());
      const key = `${file}: ${tc.name}`;
      assert.ok(!usedExclusions.has(key));
      usedExclusions.add(key);
      console.log(`NOT APPLICABLE ${key}: ${exclusion.reason}`);
      excluded++;
      continue;
    }
    try {
      await run(tc);
      console.log(`PASS ${file}: ${tc.name}`);
      passed++;
    } catch (e) {
      console.error(`FAIL ${file}: ${tc.name}`, e);
      failed++;
    }
  }
}
assert.equal(
  usedExclusions.size,
  exclusions.length,
  "Stale or duplicate applicability exclusions",
);
console.log(
  `TypeScript conformance: ${passed} passed, ${failed} failed, ${total - excluded} applicable, ${excluded} explicitly not applicable, ${total} total`,
);
if (failed) process.exitCode = 1;
