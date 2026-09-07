// @ts-check

import { execFile } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

export const capabilityBinaryEnvironment = "LLM_PROXY_BROWSER_CAPABILITY_BINARY";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const executeFile = promisify(execFile);

export default async function globalSetup() {
  const buildRoot = await mkdtemp(path.join(os.tmpdir(), "llm-proxy-browser-build-"));
  const binaryPath = path.join(buildRoot, "llm-proxy");
  const cleanup = async () => {
    delete process.env[capabilityBinaryEnvironment];
    await rm(buildRoot, { recursive: true });
  };
  try {
    await executeFile("go", ["build", "-o", binaryPath, "./cmd/cli"], { cwd: repoRoot });
    process.env[capabilityBinaryEnvironment] = binaryPath;
    return cleanup;
  } catch (buildError) {
    await cleanup();
    throw new Error("browser_capability_binary_build_failed", { cause: buildError });
  }
}
