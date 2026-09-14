// Explicit fixture identity inventory: removal cannot silently reduce conformance.
import { readdirSync, readFileSync, writeFileSync } from "node:fs";
const directory = new URL("../../tests/", import.meta.url);
const output = new URL("./fixture-inventory.json", import.meta.url);
const inventory = Object.fromEntries(
  readdirSync(directory)
    .filter((f) => f.endsWith(".json"))
    .sort()
    .map((file) => [
      file,
      JSON.parse(readFileSync(new URL(file, directory), "utf8")).map((t) => ({
        name: t.name,
        operation: t.operation,
      })),
    ]),
);
const text = JSON.stringify(inventory, null, 2) + "\n";
if (process.argv.includes("--check")) {
  if (readFileSync(output, "utf8") !== text)
    throw new Error(
      "Fixture inventory changed; review additions/removals and run node conformance/runner/typescript/generate-inventory.mjs",
    );
  console.log("Conformance fixture inventory verified");
} else writeFileSync(output, text);
