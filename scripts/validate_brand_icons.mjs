// @ts-check

import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { load } from "js-yaml";
import { brandIconManifest } from "../site/assets/llm-proxy/js/brandIconManifest.js";
import { validateBrandAssets, validateBrandMappings } from "./brand_icon_validation.mjs";

const options = new Map();
const argumentsList = process.argv.slice(2);
for (let index = 0; index < argumentsList.length; index += 2) {
  const option = argumentsList[index];
  const value = argumentsList[index + 1];
  if (!["--catalog", "--assets", "--manifest"].includes(option) || !value || options.has(option)) {
    throw new Error(`brand_icon_argument_invalid: ${option}`);
  }
  options.set(option, value);
}
const catalogPath = options.get("--catalog") ?? fileURLToPath(new URL("../configs/providers.yml", import.meta.url));
const assetPath = options.get("--assets") ?? fileURLToPath(new URL("../site/assets/llm-proxy/img/brands", import.meta.url));
const manifest = options.has("--manifest")
  ? JSON.parse(await readFile(options.get("--manifest"), "utf8"))
  : brandIconManifest;
const catalog = /** @type {Parameters<typeof validateBrandMappings>[0]} */ (load(await readFile(catalogPath, "utf8")));
validateBrandMappings(catalog, manifest);
await validateBrandAssets(assetPath, manifest);
process.stdout.write(`Brand icons verified: ${Object.keys(manifest.providers).length} providers, ${Object.keys(manifest.families).length} families, ${Object.keys(manifest.assets).length} SVGs.\n`);
