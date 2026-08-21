// @ts-check

import { readFile, writeFile } from "node:fs/promises";
import net from "node:net";
import path from "node:path";

const loopbackHost = "127.0.0.1";
const sourcePortLine = "  port: 8080";
const privateConfigMode = 0o600;
const providerCatalogFileName = "providers.yml";

const [sourceConfigPath, outputConfigPath, ...unexpectedArguments] = process.argv.slice(2);
if (!sourceConfigPath || !outputConfigPath || unexpectedArguments.length !== 0) {
  throw new Error("public_capability_test_config_arguments_invalid");
}

const capabilityPort = await reserveLoopbackPort();
const sourceConfig = await readFile(sourceConfigPath, "utf8");
const sourceProviderCatalogPath = path.join(path.dirname(sourceConfigPath), providerCatalogFileName);
const providerCatalog = await readFile(sourceProviderCatalogPath);
if (sourceConfig.split(sourcePortLine).length !== 2) {
  throw new Error(`public_capability_test_config_port_source_invalid: ${sourceConfigPath}`);
}
const capabilityConfig = sourceConfig.replace(sourcePortLine, `  port: ${capabilityPort}`);
await writeFile(outputConfigPath, capabilityConfig, {
  encoding: "utf8",
  flag: "wx",
  mode: privateConfigMode,
});
await writeFile(path.join(path.dirname(outputConfigPath), providerCatalogFileName), providerCatalog, {
  flag: "wx",
  mode: privateConfigMode,
});
process.stdout.write(`${capabilityPort}\n`);

/**
 * @returns {Promise<number>}
 */
async function reserveLoopbackPort() {
  const portServer = net.createServer();
  await new Promise((resolve, reject) => {
    portServer.once("error", reject);
    portServer.listen(0, loopbackHost, () => resolve(undefined));
  });
  const address = portServer.address();
  if (!address || typeof address === "string") {
    throw new Error("public_capability_test_config_port_missing");
  }
  await new Promise((resolve, reject) => {
    portServer.close((closeError) => closeError ? reject(closeError) : resolve(undefined));
  });
  return address.port;
}
