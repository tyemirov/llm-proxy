// @ts-check

import { brandIconManifest } from "./brandIconManifest.js?v=20260903f037";

export const brandAssetPath = "/assets/llm-proxy/img/brands/";

/** Resolve an external catalog identity to its declared brand presentation.
 * @param {string} kind
 * @param {string} identifier
 */
export function brandIcon(kind, identifier) {
  if (kind !== "provider" && kind !== "family") {
    throw new Error(`brand_icon_kind_invalid: ${kind}`);
  }
  const mappings = kind === "provider" ? brandIconManifest.providers : brandIconManifest.families;
  if (!Object.hasOwn(mappings, identifier)) {
    throw new Error(`brand_icon_mapping_missing: ${kind}=${identifier}`);
  }
  const assetID = mappings[identifier];
  return assetID === null ? null : brandIconManifest.assets[assetID];
}

/** Render a decorative image from the validated asset manifest.
 * @param {string} kind
 * @param {string} identifier
 */
export function renderBrandIcon(kind, identifier) {
  const asset = brandIcon(kind, identifier);
  if (asset === null) return "";
  return `<img class="brand-icon brand-icon--${asset.surface}" data-brand-id="${identifier}" src="${brandAssetPath}${asset.file}?v=${asset.sha256.slice(0, 12)}" width="20" height="20" alt="" aria-hidden="true">`;
}
