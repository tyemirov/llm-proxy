// @ts-check

import { readFile, writeFile } from "node:fs/promises";
import { load } from "js-yaml";

const sourcePath = "docs/openapi.yaml";
const outputPath = "internal/proxy/system_one_schema.json";
const prefix = "TypeSafe";
const document = /** @type {{components: {schemas: Record<string, unknown>}}} */ (load(await readFile(sourcePath, "utf8")));
const schemas = Object.fromEntries(Object.entries(document.components.schemas)
  .filter(([name]) => name.startsWith(prefix))
  .map(([name, schema]) => [name.slice(prefix.length), JSON.parse(JSON.stringify(schema)
    .replaceAll("#/components/schemas/TypeSafe", "#/components/schemas/"))]));
const generated = JSON.stringify({
  $schema: "https://json-schema.org/draft/2020-12/schema",
  components: { schemas },
}, null, 2) + "\n";
if (process.argv.includes("--check")) {
  if (await readFile(outputPath, "utf8") !== generated) {
    throw new Error("system_one_schema_out_of_date: run make generate-api-docs");
  }
} else {
  await writeFile(outputPath, generated);
}
