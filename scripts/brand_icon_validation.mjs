// @ts-check

import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { brandIconManifest } from "../site/assets/llm-proxy/js/brandIconManifest.js";

/** @typedef {import("../site/assets/llm-proxy/js/brandIconManifest.js").BrandIconManifest} BrandIconManifest */

/** Validate presentation data against every private catalog identity.
 * @param {{providers: {id: string}[], families: {id: string}[]}} catalog
 * @param {BrandIconManifest} manifest
 */
export function validateBrandMappings(catalog, manifest = brandIconManifest) {
  for (const kind of /** @type {const} */ (["providers", "families"])) {
    const records = catalog[kind];
    const mappings = manifest[kind];
    const expected = records.map((record) => record.id).sort();
    if (JSON.stringify(Object.keys(mappings).sort()) !== JSON.stringify(expected)) {
      throw new Error(`brand_icon_catalog_mismatch: ${kind}`);
    }
    for (const [identifier, assetID] of Object.entries(mappings)) {
      if (kind === "families" && assetID === null) continue;
      if (typeof assetID !== "string" || !Object.hasOwn(manifest.assets, assetID)) {
        throw new Error(`brand_icon_asset_reference_invalid: ${kind}=${identifier}`);
      }
    }
  }
}

/** Verify local SVGs and their pinned source records before publication.
 * @param {string} directory
 * @param {BrandIconManifest} manifest
 */
export async function validateBrandAssets(directory, manifest = brandIconManifest) {
  await readFile(join(directory, "LICENSE"));
  for (const [identifier, asset] of Object.entries(manifest.assets)) {
    if (!/^[a-z0-9-]+\.svg$/u.test(asset.file) || !/^[a-f0-9]{64}$/u.test(asset.sha256) ||
        !["light", "dark", "plain"].includes(asset.surface) ||
        !/^https:\/\/raw\.githubusercontent\.com\/lobehub\/lobe-icons\/[a-f0-9]{40}\/packages\/static-svg\/icons\/[a-z0-9-]+\.svg$/u.test(asset.source)) {
      throw new Error(`brand_icon_asset_metadata_invalid: ${identifier}`);
    }
    const content = await readFile(join(directory, asset.file));
    if (createHash("sha256").update(content).digest("hex") !== asset.sha256) {
      throw new Error(`brand_icon_asset_digest_mismatch: ${identifier}`);
    }
    const svg = content.toString("utf8");
    if (!svg.startsWith("<svg ") || /<(script|foreignObject|image|use)\b|\son\w+=|(?:href|src)=/iu.test(svg)) {
      throw new Error(`brand_icon_svg_invalid: ${identifier}`);
    }
  }
}
