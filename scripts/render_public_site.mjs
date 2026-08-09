// @ts-check

import { cp, lstat, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { extname, isAbsolute, join, relative, resolve, sep } from "node:path";

const publicCapabilitiesPath = "/api/public/capabilities";
const managementConfigPath = "/config-ui.yaml";
const sourceConfigAttribute = `data-config-url="${managementConfigPath}"`;
const routingTreeMarker = "<!-- llm-proxy-routing-tree -->";
const capabilityCatalogMarker = "<!-- llm-proxy-capability-catalog -->";
const legacyRuntimeConfigFile = "llm-proxy-config.json";
const binaryBytesPerMiB = 1024 * 1024;

const capabilityDefinitions = Object.freeze([
  { identifier: "text", label: "Text generation", className: "capability-badge--primary" },
  { identifier: "dictation", label: "Dictation", className: "capability-badge--info" },
  { identifier: "image_input", label: "Image input", className: "capability-badge--info" },
  { identifier: "audio_input", label: "Audio message input", className: "capability-badge--info" },
  { identifier: "web_search", label: "Web search", className: "capability-badge--success" },
  { identifier: "reasoning", label: "Reasoning", className: "capability-badge--success" },
]);
const capabilityDefinitionsByIdentifier = new Map(
  capabilityDefinitions.map((definition) => [definition.identifier, definition]),
);

const options = parseArguments(process.argv.slice(2));
await renderPublicSite(options);

/**
 * @typedef {{
 *   source: string,
 *   output: string,
 *   configURL: string,
 *   capabilitiesURL: string,
 * }} RenderOptions
 */

/**
 * @typedef {{
 *   identifier: string,
 *   default_endpoints: string[],
 *   capabilities: string[],
 *   wire_contract: string,
 *   output_token_limit: number,
 *   reasoning_efforts: string[],
 * }} PublicModelCapability
 */

/**
 * @typedef {{identifier: string, label: string, models: PublicModelCapability[]}} PublicProviderCapability
 */

/**
 * @typedef {{
 *   providers: PublicProviderCapability[],
 *   max_prompt_bytes: number,
 *   max_input_audio_bytes: number,
 *   max_request_timeout_seconds: number,
 * }} PublicCapabilityCatalog
 */

/**
 * @typedef {{identifier: string, label: string, className: string}} CapabilityDefinition
 */

/**
 * @param {string[]} rawArguments
 * @returns {RenderOptions}
 */
function parseArguments(rawArguments) {
  const values = new Map();
  for (let argumentIndex = 0; argumentIndex < rawArguments.length; argumentIndex += 2) {
    const option = rawArguments[argumentIndex];
    const value = rawArguments[argumentIndex + 1];
    if (!option?.startsWith("--") || value === undefined || value.startsWith("--")) {
      throw new Error(`public_site_argument_invalid: ${option ?? "missing"}`);
    }
    if (values.has(option)) {
      throw new Error(`public_site_argument_duplicate: ${option}`);
    }
    values.set(option, value);
  }
  const supportedOptions = new Set(["--source", "--output", "--config-url", "--capabilities-url"]);
  for (const option of values.keys()) {
    if (!supportedOptions.has(option)) {
      throw new Error(`public_site_argument_unknown: ${option}`);
    }
  }
  for (const option of supportedOptions) {
    if (!values.get(option)?.trim()) {
      throw new Error(`public_site_argument_required: ${option}`);
    }
  }
  return {
    source: values.get("--source")?.trim() ?? "",
    output: values.get("--output")?.trim() ?? "",
    configURL: validateConfigURL(values.get("--config-url")?.trim() ?? ""),
    capabilitiesURL: validateCapabilitiesURL(values.get("--capabilities-url")?.trim() ?? ""),
  };
}

/**
 * @param {RenderOptions} renderOptions
 */
async function renderPublicSite(renderOptions) {
  const sourceDirectory = resolve(renderOptions.source);
  const outputDirectory = resolve(renderOptions.output);
  const sourceStatus = await lstat(sourceDirectory);
  if (!sourceStatus.isDirectory()) {
    throw new Error(`public_site_source_not_directory: ${sourceDirectory}`);
  }
  const relativeOutput = relative(sourceDirectory, outputDirectory);
  if (relativeOutput === "" || (!relativeOutput.startsWith(`..${sep}`) && relativeOutput !== ".." && !isAbsolute(relativeOutput))) {
    throw new Error(`public_site_output_inside_source: source=${sourceDirectory} output=${outputDirectory}`);
  }
  if (await pathExists(outputDirectory)) {
    throw new Error(`public_site_output_exists: ${outputDirectory}`);
  }

  const capabilityCatalog = await fetchCapabilityCatalog(renderOptions.capabilitiesURL);
  await cp(sourceDirectory, outputDirectory, { recursive: true, errorOnExist: true, force: false });
  await rm(join(outputDirectory, managementConfigPath.slice(1)), { force: true });
  await rm(join(outputDirectory, legacyRuntimeConfigFile), { force: true });
  await renderConfigURLs(outputDirectory, renderOptions.configURL);
  await renderLandingContent(outputDirectory, capabilityCatalog);
  if (!await pathExists(join(outputDirectory, "CNAME"))) {
    throw new Error(`public_site_cname_missing: ${outputDirectory}`);
  }
}

/**
 * @param {string} capabilitiesURL
 * @returns {Promise<PublicCapabilityCatalog>}
 */
async function fetchCapabilityCatalog(capabilitiesURL) {
  const response = await fetch(capabilitiesURL, {
    headers: { Accept: "application/json" },
    signal: AbortSignal.timeout(10_000),
  });
  if (!response.ok) {
    throw new Error(`public_capabilities_request_failed: status=${response.status}`);
  }
  return parseCapabilityCatalog(await response.json());
}

/**
 * @param {unknown} rawCatalog
 * @returns {PublicCapabilityCatalog}
 */
function parseCapabilityCatalog(rawCatalog) {
  const catalog = requiredRecord(rawCatalog, "catalog");
  requireExactKeys(catalog, [
    "providers",
    "max_prompt_bytes",
    "max_input_audio_bytes",
    "max_request_timeout_seconds",
  ], "catalog");
  const rawProviders = requiredArray(catalog.providers, "catalog.providers");
  if (rawProviders.length === 0) {
    throw new Error("public_capabilities_invalid: catalog.providers must not be empty");
  }
  const providers = rawProviders.map((rawProvider, providerIndex) => {
    const field = `catalog.providers[${providerIndex}]`;
    const provider = requiredRecord(rawProvider, field);
    requireExactKeys(provider, ["identifier", "label", "models"], field);
    const rawModels = requiredArray(provider.models, `${field}.models`);
    if (rawModels.length === 0) {
      throw new Error(`public_capabilities_invalid: ${field}.models must not be empty`);
    }
    return {
      identifier: requiredString(provider.identifier, `${field}.identifier`),
      label: requiredString(provider.label, `${field}.label`),
      models: rawModels.map((rawModel, modelIndex) => parsePublicModel(rawModel, `${field}.models[${modelIndex}]`)),
    };
  });
  return {
    providers,
    max_prompt_bytes: requiredPositiveInteger(catalog.max_prompt_bytes, "catalog.max_prompt_bytes"),
    max_input_audio_bytes: requiredPositiveInteger(catalog.max_input_audio_bytes, "catalog.max_input_audio_bytes"),
    max_request_timeout_seconds: requiredPositiveInteger(
      catalog.max_request_timeout_seconds,
      "catalog.max_request_timeout_seconds",
    ),
  };
}

/**
 * @param {unknown} rawModel
 * @param {string} field
 * @returns {PublicModelCapability}
 */
function parsePublicModel(rawModel, field) {
  const model = requiredRecord(rawModel, field);
  requireExactKeys(model, [
    "identifier",
    "default_endpoints",
    "capabilities",
    "wire_contract",
    "output_token_limit",
    "reasoning_efforts",
  ], field);
  const capabilities = requiredStringArray(model.capabilities, `${field}.capabilities`);
  if (capabilities.length === 0) {
    throw new Error(`public_capabilities_invalid: ${field}.capabilities must not be empty`);
  }
  for (const capability of capabilities) {
    if (!capabilityDefinitionsByIdentifier.has(capability)) {
      throw new Error(`public_capabilities_invalid: ${field}.capabilities value=${capability}`);
    }
  }
  const defaultEndpoints = requiredStringArray(model.default_endpoints, `${field}.default_endpoints`);
  for (const endpoint of defaultEndpoints) {
    if (endpoint !== "text" && endpoint !== "dictation") {
      throw new Error(`public_capabilities_invalid: ${field}.default_endpoints value=${endpoint}`);
    }
  }
  return {
    identifier: requiredString(model.identifier, `${field}.identifier`),
    default_endpoints: defaultEndpoints,
    capabilities,
    wire_contract: requiredString(model.wire_contract, `${field}.wire_contract`, true),
    output_token_limit: requiredNonnegativeInteger(model.output_token_limit, `${field}.output_token_limit`),
    reasoning_efforts: requiredStringArray(model.reasoning_efforts, `${field}.reasoning_efforts`),
  };
}

/**
 * @param {string} outputDirectory
 * @param {string} configURL
 */
async function renderConfigURLs(outputDirectory, configURL) {
  for (const htmlPath of await filesUnder(outputDirectory, ".html")) {
    const sourceHTML = await readFile(htmlPath, "utf8");
    if (countOccurrences(sourceHTML, sourceConfigAttribute) !== 1) {
      throw new Error(`public_site_config_attribute_invalid: path=${htmlPath}`);
    }
    await writeFile(
      htmlPath,
      sourceHTML.replace(sourceConfigAttribute, `data-config-url="${escapeAttribute(configURL)}"`),
      "utf8",
    );
  }
}

/**
 * @param {string} outputDirectory
 * @param {PublicCapabilityCatalog} capabilityCatalog
 */
async function renderLandingContent(outputDirectory, capabilityCatalog) {
  const landingPath = join(outputDirectory, "index.html");
  const sourceHTML = await readFile(landingPath, "utf8");
  for (const marker of [routingTreeMarker, capabilityCatalogMarker]) {
    if (countOccurrences(sourceHTML, marker) !== 1) {
      throw new Error(`public_site_marker_invalid: path=${landingPath} marker=${marker}`);
    }
  }
  const renderedHTML = sourceHTML
    .replace(routingTreeMarker, renderRoutingTree(capabilityCatalog))
    .replace(capabilityCatalogMarker, renderCapabilityCatalog(capabilityCatalog));
  await writeFile(landingPath, renderedHTML, "utf8");
}

/**
 * @param {PublicCapabilityCatalog} catalog
 * @returns {string}
 */
function renderRoutingTree(catalog) {
  const providers = catalog.providers.map((provider) => ({
    ...provider,
    models: provider.models.filter((model) => model.capabilities.includes("text")),
  }));
  for (const provider of providers) {
    if (provider.models.length === 0) {
      throw new Error(`public_capabilities_invalid: provider=${provider.identifier} has no text models`);
    }
  }
  const selectedProvider = providers[0];
  const selectedModel = selectedProvider.models.find((model) => model.default_endpoints.includes("text"));
  if (!selectedModel) {
    throw new Error(`public_capabilities_invalid: provider=${selectedProvider.identifier} has no default text model`);
  }
  const modelCount = providers.reduce((count, provider) => count + provider.models.length, 0);
  const providerButtons = providers.map((provider, providerIndex) => `        <button type="button" class="routing-tree__branch routing-tree__provider" data-route-provider="${escapeAttribute(provider.identifier)}" aria-controls="routing-tree-models-${escapeAttribute(provider.identifier)}" aria-pressed="${providerIndex === 0 ? "true" : "false"}" disabled>
          <strong>${escapeHTML(provider.label)}</strong><small>${provider.models.length} models</small>
        </button>`).join("");
  const modelGroups = providers.map((provider, providerIndex) => {
    const modelButtons = provider.models.map((model) => {
      const isDefault = model.default_endpoints.includes("text");
      const isSelected = providerIndex === 0 && model.identifier === selectedModel.identifier;
      return `<button type="button" class="routing-tree__branch routing-tree__model" data-route-model="${escapeAttribute(model.identifier)}"${isDefault ? " data-route-default-model=\"true\"" : ""} aria-pressed="${isSelected ? "true" : "false"}" disabled><code>${escapeHTML(model.identifier)}</code>${isDefault ? "<small>Provider default</small>" : ""}</button>`;
    }).join("");
    return `      <section id="routing-tree-models-${escapeAttribute(provider.identifier)}" class="routing-tree__model-group" data-route-model-group="${escapeAttribute(provider.identifier)}" aria-label="${escapeAttribute(provider.label)} supported text models"${providerIndex === 0 ? "" : " hidden"}>
        <p><strong>${escapeHTML(provider.label)}</strong><span>${provider.models.length} supported text models</span></p>
        <div class="routing-tree__branches routing-tree__model-branches" role="group" aria-label="Choose a ${escapeAttribute(provider.label)} model">
          ${modelButtons}
        </div>
      </section>`;
  }).join("");
  return `<routing-tree class="routing-tree" data-enhanced="false" aria-label="Interactive LLM routing map">
  <header class="routing-tree__header">
    <h2>One integration. Choose the exact route.</h2>
    <span>${providers.length} providers · ${modelCount} text models</span>
  </header>
  <div class="routing-tree__map" data-route-map>
    <canvas class="routing-tree__connectors" data-route-canvas aria-hidden="true"></canvas>
    <div class="routing-tree__ingress" aria-label="One connection into LLM Proxy">
      <article class="routing-tree__node routing-tree__node--product" data-route-product>
        <strong>Your product</strong>
        <span>HTTP · Go · Python · CLI</span>
      </article>
      <article class="routing-tree__node routing-tree__node--proxy" data-route-proxy>
        <strong>LLM Proxy</strong>
        <span>Authenticate · validate · route</span>
      </article>
    </div>
    <section class="routing-tree__stage routing-tree__stage--providers" aria-labelledby="routing-tree-provider-title">
      <h3 id="routing-tree-provider-title" class="routing-tree__stage-title">Choose a provider</h3>
      <div class="routing-tree__branches routing-tree__provider-branches" role="group" aria-label="Supported providers">
${providerButtons}
      </div>
    </section>
    <section class="routing-tree__stage routing-tree__stage--models" aria-labelledby="routing-tree-model-title">
      <h3 id="routing-tree-model-title" class="routing-tree__stage-title">Choose an exact model and version</h3>
${modelGroups}
    </section>
    <footer class="routing-tree__selection">
      <span>Selected route:</span>
      <output aria-live="polite"><code data-route-selected-provider>${escapeHTML(selectedProvider.identifier)}</code><i aria-hidden="true">/</i><code data-route-selected-model>${escapeHTML(selectedModel.identifier)}</code></output>
    </footer>
  </div>
</routing-tree>`;
}

/**
 * @param {PublicCapabilityCatalog} catalog
 * @returns {string}
 */
function renderCapabilityCatalog(catalog) {
  const models = catalog.providers.flatMap((provider) => provider.models.map((model) => ({ provider, model })));
  const availableCapabilities = new Set(models.flatMap(({ model }) => model.capabilities));
  const filters = capabilityDefinitions.filter((definition) => availableCapabilities.has(definition.identifier));
  const filterControls = filters.map((definition) => `<label class="catalog-filter"><input type="checkbox" name="catalog-capability" value="${escapeAttribute(definition.identifier)}" data-catalog-capability><span>${escapeHTML(definition.label)}</span></label>`).join("");
  const rows = models.map(({ provider, model }) => renderCapabilityRow(provider, model)).join("");
  return `<capability-catalog data-enhanced="false">
  <div class="catalog-summary" aria-label="Catalog summary">
    <p><strong>${catalog.providers.length}</strong><span>Providers</span></p>
    <p><strong>${models.length}</strong><span>Models</span></p>
    <p><strong>${filters.length}</strong><span>Filterable capabilities</span></p>
  </div>
  <form class="catalog-toolbar" data-catalog-toolbar role="search" aria-label="Search and filter models">
    <div class="catalog-search-row">
      <input type="search" name="catalog-search" autocomplete="off" aria-label="Search all model characteristics" placeholder="Search provider, model, capability, contract, or limit" data-catalog-search>
      <button type="button" class="catalog-search-submit" aria-label="Toggle capability filters" aria-controls="catalog-capability-filters" aria-expanded="false" data-catalog-search-submit>
        <svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="10.5" cy="10.5" r="6.5"></circle><path d="m15.5 15.5 4.5 4.5"></path></svg>
      </button>
    </div>
    <div id="catalog-capability-filters" class="catalog-filter-panel" data-catalog-filter-panel hidden>
      <fieldset class="catalog-filter-group">
        <legend>Capabilities <span>Match all selected</span></legend>
        <div class="catalog-filters">
          ${filterControls}
        </div>
      </fieldset>
      <div class="catalog-toolbar__status">
        <output aria-live="polite" data-catalog-result-count>${models.length} models</output>
        <button type="reset" data-catalog-reset>Reset</button>
      </div>
    </div>
  </form>
  <div class="catalog-table-wrap" tabindex="0" role="region" aria-label="Provider and model capability matrix">
    <table class="catalog-table">
      <caption>Current model capabilities generated from the validated LLM Proxy provider registry.</caption>
      <thead>
        <tr>
          <th scope="col" aria-sort="ascending" data-catalog-sort-header="provider"><button type="button" class="catalog-sort-button" data-catalog-sort="provider" data-sort-label="Provider" disabled>Provider<span class="catalog-sort-indicator" aria-hidden="true"></span></button></th>
          <th scope="col" data-catalog-sort-header="model"><button type="button" class="catalog-sort-button" data-catalog-sort="model" data-sort-label="Model" disabled>Model<span class="catalog-sort-indicator" aria-hidden="true"></span></button></th>
          <th scope="col" data-catalog-sort-header="capabilities"><button type="button" class="catalog-sort-button" data-catalog-sort="capabilities" data-sort-label="Capabilities" disabled>Capabilities<span class="catalog-sort-indicator" aria-hidden="true"></span></button></th>
        </tr>
      </thead>
      <tbody data-catalog-body>
        ${rows}
      </tbody>
    </table>
    <p class="catalog-empty" data-catalog-empty hidden>No models match the selected filters.</p>
  </div>
</capability-catalog>
<div class="catalog-limits" aria-label="Proxy request limits">
  <p><strong>${formatBinarySize(catalog.max_prompt_bytes)}</strong>Maximum JSON request body</p>
  <p><strong>${formatBinarySize(catalog.max_input_audio_bytes)}</strong>Maximum input audio</p>
  <p><strong>${catalog.max_request_timeout_seconds} seconds</strong>Maximum request work budget</p>
</div>`;
}

/**
 * @param {PublicProviderCapability} provider
 * @param {PublicModelCapability} model
 * @returns {string}
 */
function renderCapabilityRow(provider, model) {
  const definitions = capabilityDefinitions.filter((definition) => model.capabilities.includes(definition.identifier));
  const defaults = defaultDefinitions(model.default_endpoints);
  const outputLimit = outputLimitLabel(model);
  const searchText = [
    provider.identifier,
    provider.label,
    model.identifier,
    model.wire_contract,
    String(model.output_token_limit),
    outputLimit,
    ...model.default_endpoints,
    ...defaults.flatMap((definition) => [definition.label, definition.description]),
    ...model.capabilities,
    ...model.reasoning_efforts,
    ...definitions.map((definition) => definition.label),
  ].join(" ");
  const defaultBadges = defaults.map((definition) => `<span class="catalog-model__default" title="${escapeAttribute(definition.description)}">${escapeHTML(definition.label)}</span>`).join("");
  const capabilityBadges = definitions.map((definition) => `<button type="button" class="capability-badge ${escapeAttribute(definition.className)}" aria-label="Filter by ${escapeAttribute(definition.label)}" data-catalog-capability-action="${escapeAttribute(definition.identifier)}" disabled>${escapeHTML(definition.label)}</button>`).join("");
  const technicalDetails = [
    model.wire_contract ? `<code>${escapeHTML(model.wire_contract)}</code>` : "",
    model.reasoning_efforts.length > 0 ? `<span>Reasoning: ${escapeHTML(model.reasoning_efforts.join(", "))}</span>` : "",
    outputLimit ? `<span>${escapeHTML(outputLimit)}</span>` : "",
  ].join("");
  return `<tr data-catalog-row data-provider="${escapeAttribute(provider.identifier)}" data-model="${escapeAttribute(model.identifier)}" data-capabilities="${escapeAttribute(model.capabilities.join(" "))}" data-capability-count="${definitions.length}" data-catalog-search-text="${escapeAttribute(searchText)}">
          <td class="catalog-provider"><strong>${escapeHTML(provider.label)}</strong><code>${escapeHTML(provider.identifier)}</code></td>
          <td class="catalog-model"><span class="catalog-model__content"><code data-catalog-model-id>${escapeHTML(model.identifier)}</code>${defaultBadges}</span></td>
          <td><div class="catalog-capabilities">
            ${capabilityBadges}
          </div>
          <div class="catalog-technical">
            ${technicalDetails}
          </div></td>
        </tr>`;
}

/**
 * @param {string[]} endpoints
 */
function defaultDefinitions(endpoints) {
  const definitions = [];
  if (endpoints.includes("text")) {
    definitions.push({
      label: "Default for text",
      description: "This is the provider catalog default for text routing; account settings can select another model.",
    });
  }
  if (endpoints.includes("dictation")) {
    definitions.push({
      label: "Default for dictation",
      description: "This is the provider catalog default for dictation routing; account settings can select another model.",
    });
  }
  return definitions;
}

/**
 * @param {PublicModelCapability} model
 */
function outputLimitLabel(model) {
  if (!model.capabilities.includes("text")) {
    return "";
  }
  if (model.output_token_limit === 0) {
    return "Provider-enforced output";
  }
  return `${model.output_token_limit} token output`;
}

/**
 * @param {number} byteCount
 */
function formatBinarySize(byteCount) {
  if (byteCount % binaryBytesPerMiB === 0) {
    return `${byteCount / binaryBytesPerMiB} MiB`;
  }
  return `${byteCount} bytes`;
}

/**
 * @param {string} root
 * @param {string} extension
 * @returns {Promise<string[]>}
 */
async function filesUnder(root, extension) {
  const entries = await readdir(root, { withFileTypes: true });
  const paths = [];
  for (const entry of entries) {
    const entryPath = join(root, entry.name);
    if (entry.isDirectory()) {
      paths.push(...await filesUnder(entryPath, extension));
    } else if (extname(entry.name) === extension) {
      paths.push(entryPath);
    }
  }
  return paths;
}

/**
 * @param {string} rawURL
 */
function validateConfigURL(rawURL) {
  if (rawURL === managementConfigPath) {
    return rawURL;
  }
  const parsedURL = new URL(rawURL);
  if (parsedURL.protocol !== "https:" || !parsedURL.hostname || parsedURL.pathname !== managementConfigPath || parsedURL.search || parsedURL.hash || parsedURL.username || parsedURL.password) {
    throw new Error(`public_site_config_url_invalid: ${rawURL}`);
  }
  return parsedURL.href;
}

/**
 * @param {string} rawURL
 */
function validateCapabilitiesURL(rawURL) {
  const parsedURL = new URL(rawURL);
  if ((parsedURL.protocol !== "http:" && parsedURL.protocol !== "https:") || !parsedURL.hostname || parsedURL.pathname !== publicCapabilitiesPath || parsedURL.search || parsedURL.hash || parsedURL.username || parsedURL.password) {
    throw new Error(`public_capabilities_url_invalid: ${rawURL}`);
  }
  return parsedURL.href;
}

/**
 * @param {unknown} value
 * @param {string} field
 * @returns {Record<string, unknown>}
 */
function requiredRecord(value, field) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`public_capabilities_invalid: ${field} must be an object`);
  }
  return /** @type {Record<string, unknown>} */ (value);
}

/**
 * @param {unknown} value
 * @param {string} field
 * @returns {unknown[]}
 */
function requiredArray(value, field) {
  if (!Array.isArray(value)) {
    throw new Error(`public_capabilities_invalid: ${field} must be an array`);
  }
  return value;
}

/**
 * @param {unknown} value
 * @param {string} field
 * @param {boolean} [allowEmpty]
 */
function requiredString(value, field, allowEmpty = false) {
  if (typeof value !== "string" || (!allowEmpty && !value.trim())) {
    throw new Error(`public_capabilities_invalid: ${field} must be ${allowEmpty ? "a string" : "a nonempty string"}`);
  }
  return value;
}

/**
 * @param {unknown} value
 * @param {string} field
 * @returns {string[]}
 */
function requiredStringArray(value, field) {
  return requiredArray(value, field).map((entry, index) => requiredString(entry, `${field}[${index}]`));
}

/**
 * @param {unknown} value
 * @param {string} field
 */
function requiredPositiveInteger(value, field) {
  const integer = requiredNonnegativeInteger(value, field);
  if (integer === 0) {
    throw new Error(`public_capabilities_invalid: ${field} must be positive`);
  }
  return integer;
}

/**
 * @param {unknown} value
 * @param {string} field
 */
function requiredNonnegativeInteger(value, field) {
  if (!Number.isSafeInteger(value) || Number(value) < 0) {
    throw new Error(`public_capabilities_invalid: ${field} must be a nonnegative integer`);
  }
  return Number(value);
}

/**
 * @param {Record<string, unknown>} value
 * @param {string[]} expectedKeys
 * @param {string} field
 */
function requireExactKeys(value, expectedKeys, field) {
  const actualKeys = Object.keys(value).sort();
  const sortedExpectedKeys = [...expectedKeys].sort();
  if (actualKeys.length !== sortedExpectedKeys.length || actualKeys.some((key, index) => key !== sortedExpectedKeys[index])) {
    throw new Error(`public_capabilities_invalid: ${field} keys=${actualKeys.join(",")}`);
  }
}

/**
 * @param {string} value
 */
function escapeHTML(value) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

/**
 * @param {string} value
 */
function escapeAttribute(value) {
  return escapeHTML(value).replaceAll("\n", "&#10;").replaceAll("\r", "&#13;");
}

/**
 * @param {string} value
 * @param {string} search
 */
function countOccurrences(value, search) {
  return value.split(search).length - 1;
}

/**
 * @param {string} path
 */
async function pathExists(path) {
  try {
    await lstat(path);
    return true;
  } catch (error) {
    if (error && typeof error === "object" && "code" in error && error.code === "ENOENT") {
      return false;
    }
    throw error;
  }
}
