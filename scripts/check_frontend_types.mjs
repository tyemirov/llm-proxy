// @ts-check

import { mkdir, mkdtemp, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));
const mirrorParent = join(repositoryRoot, "node_modules", ".cache");
const browserSourceRoot = join("site", "assets", "llm-proxy", "js");
const sourceRoots = ["scripts", browserSourceRoot];
const cacheBustedImportPattern = /(\.js)\?v=[0-9a-z]+(?=["'])/g;
const alpineCDNImport = "https://cdn.jsdelivr.net/npm/alpinejs@3.17.1/dist/module.esm.js";
const alpineTypecheckImport = "./alpineRuntimeDependency.js";
const alpineTypecheckDeclaration = `declare const Alpine: {
  data(name: string, factory: () => object): void;
  start(): void;
};
export default Alpine;
`;

await mkdir(mirrorParent, { recursive: true });
const mirrorRoot = await mkdtemp(join(mirrorParent, "llm-proxy-types-"));
let typecheckStatus = 1;
try {
  await copyTypecheckSource("jsconfig.json");
  for (const sourceRoot of sourceRoots) {
    for (const sourcePath of await javascriptFiles(sourceRoot)) {
      await copyTypecheckSource(sourcePath);
    }
  }
  await writeTypecheckSource(
    join(browserSourceRoot, "alpineRuntimeDependency.d.ts"),
    alpineTypecheckDeclaration,
  );
  const typecheckResult = spawnSync(
    join(repositoryRoot, "node_modules", ".bin", "tsc"),
    ["--noEmit", "--project", join(mirrorRoot, "jsconfig.json")],
    { cwd: mirrorRoot, stdio: "inherit" },
  );
  if (typecheckResult.error) {
    throw new Error(`frontend_typecheck_start_failed: ${typecheckResult.error.message}`);
  }
  typecheckStatus = typecheckResult.status ?? 1;
} finally {
  await rm(mirrorRoot, { recursive: true });
}
process.exitCode = typecheckStatus;

/**
 * @param {string} relativePath
 */
async function copyTypecheckSource(relativePath) {
  const sourcePath = join(repositoryRoot, relativePath);
  const source = await readFile(sourcePath, "utf8");
  const normalizedSource = relativePath.startsWith(`${browserSourceRoot}/`)
    ? source
      .replace(cacheBustedImportPattern, "$1")
      .replaceAll(alpineCDNImport, alpineTypecheckImport)
    : source;
  await writeTypecheckSource(relativePath, normalizedSource);
}

/**
 * @param {string} relativePath
 * @param {string} source
 */
async function writeTypecheckSource(relativePath, source) {
  const destinationPath = join(mirrorRoot, relativePath);
  await mkdir(dirname(destinationPath), { recursive: true });
  await writeFile(destinationPath, source, "utf8");
}

/**
 * @param {string} relativeDirectory
 * @returns {Promise<string[]>}
 */
async function javascriptFiles(relativeDirectory) {
  const entries = await readdir(join(repositoryRoot, relativeDirectory), { withFileTypes: true });
  const paths = [];
  for (const entry of entries) {
    const relativePath = join(relativeDirectory, entry.name);
    if (entry.isDirectory()) {
      paths.push(...await javascriptFiles(relativePath));
    } else if (entry.name.endsWith(".js") || entry.name.endsWith(".mjs")) {
      paths.push(relativePath);
    }
  }
  return paths;
}
