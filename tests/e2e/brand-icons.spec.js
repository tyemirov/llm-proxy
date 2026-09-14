// @ts-check

import { expect, test } from "@playwright/test";
import { execFile } from "node:child_process";
import { cp, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { promisify } from "node:util";
import { fileURLToPath } from "node:url";
import { load, dump } from "js-yaml";
import { brandIconManifest } from "../../site/assets/llm-proxy/js/brandIconManifest.js";

const executeFile = promisify(execFile);
const root = fileURLToPath(new URL("../..", import.meta.url));

test("brand icons build validates the complete private catalog", async () => {
  const result = await executeFile("node", ["scripts/validate_brand_icons.mjs"], { cwd: root });
  expect(result.stdout).toContain("14 providers, 30 families, 18 SVGs");
});

for (const scenario of ["unknown provider", "unknown family", "obsolete mapping", "missing asset", "changed asset", "invalid reference", "invalid path"]) {
  test(`brand icons build rejects ${scenario}`, async () => {
    const directory = await mkdtemp(path.join(tmpdir(), "llm-proxy-brands-"));
    try {
      const catalog = load(await readFile(path.join(root, "configs/providers.yml"), "utf8"));
      const manifest = structuredClone(brandIconManifest);
      const assets = path.join(directory, "assets");
      await cp(path.join(root, "site/assets/llm-proxy/img/brands"), assets, { recursive: true });
      let message = "";
      switch (scenario) {
        case "unknown provider":
          catalog.providers.push({ id: "new-provider" });
          message = "brand_icon_catalog_mismatch: providers";
          break;
        case "unknown family":
          catalog.families.push({ id: "new-family" });
          message = "brand_icon_catalog_mismatch: families";
          break;
        case "obsolete mapping":
          manifest.providers.obsolete = "openai";
          message = "brand_icon_catalog_mismatch: providers";
          break;
        case "missing asset":
          await rm(path.join(assets, "openai.svg"));
          message = "openai.svg";
          break;
        case "changed asset":
          await writeFile(path.join(assets, "openai.svg"), "<svg></svg>");
          message = "brand_icon_asset_digest_mismatch: openai";
          break;
        case "invalid reference":
          manifest.families.gemini = "missing";
          message = "brand_icon_asset_reference_invalid: families=gemini";
          break;
        case "invalid path":
          manifest.assets.openai.file = "../openai.svg";
          message = "brand_icon_asset_metadata_invalid: openai";
          break;
      }
      const catalogPath = path.join(directory, "providers.yml");
      const manifestPath = path.join(directory, "manifest.json");
      await writeFile(catalogPath, dump(catalog));
      await writeFile(manifestPath, JSON.stringify(manifest));
      await expect(executeFile("node", ["scripts/validate_brand_icons.mjs", "--catalog", catalogPath, "--assets", assets, "--manifest", manifestPath], { cwd: root })).rejects.toThrow(message);
    } finally {
      await rm(directory, { recursive: true, force: true });
    }
  });
}
