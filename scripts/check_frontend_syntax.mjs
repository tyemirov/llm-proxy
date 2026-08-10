// @ts-check

import { readFileSync, readdirSync } from "node:fs";
import { extname, join } from "node:path";
import { spawnSync } from "node:child_process";

const files = [
  "playwright.config.js",
  "playwright.blackbox.config.js",
  "scripts/create_public_capability_test_config.mjs",
  "scripts/generate_openapi_docs.mjs",
  "scripts/generate_legal_pages.mjs",
  "scripts/generate_public_site_shell.mjs",
  "scripts/render_public_site.mjs",
  "scripts/generate_seo_resources.mjs",
  "scripts/public_site_shell.mjs",
  "tests/blackbox/localManagementStack.mjs",
];
const roots = ["site/assets/llm-proxy/js", "tests/e2e", "tests/blackbox"];
const javascriptExtension = ".js";
const browserTextExtensions = new Set([".css", ".html", ".js", ".xml"]);
const obsoletePublicTermPattern = new RegExp(["work", "space"].join(""), "i");

for (const file of files) {
  checkSyntax(file);
}
for (const root of roots) {
  for (const file of javascriptFiles(root)) {
    checkSyntax(file);
  }
}
for (const file of browserTextFiles("site")) {
  if (obsoletePublicTermPattern.test(readFileSync(file, "utf8"))) {
    throw new Error(`obsolete_public_terminology: ${file}`);
  }
}

/**
 * @param {string} file
 */
function checkSyntax(file) {
  const result = spawnSync(process.execPath, ["--check", file], { stdio: "inherit" });
  if (result.status !== 0) {
    process.exit(result.status || 1);
  }
}

/**
 * @param {string} directory
 * @returns {string[]}
 */
function javascriptFiles(directory) {
  const entries = readdirSync(directory, { withFileTypes: true });
  return entries.flatMap((entry) => {
    const entryPath = join(directory, entry.name);
    if (entry.isDirectory()) {
      return javascriptFiles(entryPath);
    }
    return entry.name.endsWith(javascriptExtension) ? [entryPath] : [];
  });
}

/**
 * @param {string} directory
 * @returns {string[]}
 */
function browserTextFiles(directory) {
  const entries = readdirSync(directory, { withFileTypes: true });
  return entries.flatMap((entry) => {
    const entryPath = join(directory, entry.name);
    if (entry.isDirectory()) {
      return browserTextFiles(entryPath);
    }
    return browserTextExtensions.has(extname(entry.name)) ? [entryPath] : [];
  });
}
