import { readdirSync } from "node:fs";
import { join } from "node:path";
import { spawnSync } from "node:child_process";

const files = ["playwright.config.js"];
const roots = ["site/assets/llm-proxy/js", "tests/e2e"];
const javascriptExtension = ".js";

for (const file of files) {
  checkSyntax(file);
}
for (const root of roots) {
  for (const file of javascriptFiles(root)) {
    checkSyntax(file);
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
