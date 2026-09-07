// HEY adaptation of Basecamp's OpenAPI type, metadata and service generators.
// No account path stripping: HEY's explicit accountId labels are real route inputs.
import { readFile, writeFile, mkdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import openapiTS, { astToString, type OpenAPI3 } from "openapi-typescript";
import ts from "typescript";

type Schema = {
  $ref?: string;
  type?: string;
  properties?: Record<string, Schema>;
  [key: string]: unknown;
};
type Operation = {
  operationId: string;
  description?: string;
  parameters?: Array<{
    name: string;
    in: string;
    required?: boolean;
    schema: Schema;
  }>;
  requestBody?: {
    required?: boolean;
    content: Record<string, { schema: Schema }>;
  };
  [key: string]: unknown;
};
export interface Spec {
  paths: Record<string, Record<string, Operation>>;
  components: { schemas: Record<string, Schema> };
}
export interface Behavior {
  operations: Record<string, { readonly: boolean; idempotent: boolean }>;
}
const banner =
  "// Generated from openapi.json and behavior-model.json. Do not edit.\n";
export function generateOperations(spec: Spec, behavior: Behavior) {
  const metadata: Record<string, unknown> = {};
  const methods: string[] = [];
  const schema = (s: Schema): Schema =>
    s.$ref ? spec.components.schemas[s.$ref.split("/").at(-1)!]! : s;
  for (const [path, item] of Object.entries(spec.paths)) {
    for (const [method, op] of Object.entries(item)) {
      if (
        !["get", "post", "put", "patch", "delete", "head", "options"].includes(
          method,
        )
      )
        continue;
      const id = op.operationId;
      if (!id || metadata[id] || !behavior.operations[id])
        throw new Error(`Missing/duplicate operation or behavior: ${id}`);
      const body = op.requestBody;
      const media = body ? Object.keys(body.content) : [];
      if (media.some((m) => m !== "application/json"))
        throw new Error(`Unsupported request media for ${id}: ${media}`);
      const properties = body
        ? schema(body.content["application/json"]!.schema).properties
        : undefined;
      metadata[id] = {
        method: method.toUpperCase(),
        path,
        parameters: (op.parameters ?? []).map((p) => ({
          name: p.name,
          in: p.in,
          required: !!p.required,
          type: p.schema.type,
        })),
        bodyRequired: !!body?.required,
        safe:
          (op["x-hey-idempotent"] as { natural?: boolean } | undefined)
            ?.natural ??
          (behavior.operations[id]!.readonly ||
            behavior.operations[id]!.idempotent),
        retry: op["x-hey-retry"],
        pagination: op["x-hey-pagination"],
        emptyOn: (op["x-hey-empty-on"] as { statusCodes: number[] } | undefined)
          ?.statusCodes,
        actingSender: !!properties?.acting_sender_id,
        actingUser: !!properties?.acting_user_id,
      };
      const required = body?.required || op.parameters?.some((p) => p.required);
      const name = id[0]!.toLowerCase() + id.slice(1);
      const description = (op.description ?? id)
        .replaceAll("*/", "* /")
        .split("\n")
        .map((line) => line.trimEnd())
        .join("\n   * ");
      methods.push(
        `  /** ${description} */\n  ${name}(input: OperationInput<${JSON.stringify(id)}>${required ? "" : " = {}"}, options?: RequestOptions): Promise<OperationResponse<${JSON.stringify(id)}>> {\n    return this.transport.execute(${JSON.stringify(id)}, input, options);\n  }`,
      );
    }
  }
  const ids = Object.keys(metadata).sort();
  if (
    JSON.stringify(ids) !==
    JSON.stringify(Object.keys(behavior.operations).sort())
  )
    throw new Error("Operation coverage differs from behavior model");
  const guards: string[] = [];
  for (const [name, s] of Object.entries(spec.components.schemas)) {
    const poly = s["x-hey-polymorphic"] as
      | { discriminator: string; variants: Record<string, string[]> }
      | undefined;
    if (!poly) continue;
    for (const variant of Object.keys(poly.variants)) {
      guards.push(
        `export function is${name}${variant[0]!.toUpperCase() + variant.slice(1)}(value: components['schemas'][${JSON.stringify(name)}]): value is components['schemas'][${JSON.stringify(name)}] & { ${JSON.stringify(poly.discriminator)}: ${JSON.stringify(variant)} } {\n  return value[${JSON.stringify(poly.discriminator)}] === ${JSON.stringify(variant)};\n}`,
      );
    }
  }
  return {
    "operations.ts": `${banner}import type { OperationInput, OperationResponse, RequestOptions, Transport } from '../protocol.js';\n\nexport const operationMetadata = ${JSON.stringify(metadata, null, 2)} as const;\nexport type OperationName = keyof typeof operationMetadata;\n\nexport class GeneratedOperations {\n  constructor(protected transport: Transport) {}\n${methods.join("\n\n")}\n}\n`,
    "guards.ts": `${banner}import type { components } from './schema.js';\n${guards.join("\n\n")}\n`,
    "coverage.json":
      JSON.stringify({ generated: true, operations: ids }, null, 2) + "\n",
  };
}

export async function generate(check = false) {
  const root = new URL("../../", import.meta.url);
  const spec = JSON.parse(
    await readFile(new URL("openapi.json", root), "utf8"),
  ) as Spec;
  const behavior = JSON.parse(
    await readFile(new URL("behavior-model.json", root), "utf8"),
  ) as Behavior;
  const ast = await openapiTS(spec as unknown as OpenAPI3, {
    transform(s) {
      if (s.type === "integer" && s.format === "int64") {
        const types = [
          ts.factory.createKeywordTypeNode(ts.SyntaxKind.NumberKeyword),
          ts.factory.createKeywordTypeNode(ts.SyntaxKind.BigIntKeyword),
        ];
        if (s.nullable)
          types.push(
            ts.factory.createLiteralTypeNode(ts.factory.createNull()) as never,
          );
        return ts.factory.createUnionTypeNode(types);
      }
    },
  });
  const output = {
    "schema.ts": banner + astToString(ast),
    ...generateOperations(spec, behavior),
  };
  const directory = new URL("../src/generated/", import.meta.url);
  await mkdir(directory, { recursive: true });
  for (const [name, content] of Object.entries(output)) {
    const target = new URL(name, directory);
    const normalized = content.replace(/[ \t]+$/gm, "");
    if (check) {
      if ((await readFile(target, "utf8")) !== normalized)
        throw new Error(`Stale ${fileURLToPath(target)}; run make ts-generate`);
    } else await writeFile(target, normalized);
  }
  console.log(
    `${check ? "Verified" : "Generated"} ${Object.keys(behavior.operations).length} HEY operations, types, guards and coverage`,
  );
}
if (process.argv[1] === fileURLToPath(import.meta.url))
  await generate(process.argv.includes("--check"));
