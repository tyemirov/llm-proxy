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

/** @typedef {{identifier: string, label: string}} PublicProviderCapability */
/** @typedef {{identifier: string, label: string, model_count: number}} PublicModelPublisher */
/** @typedef {{identifier: string, publisher: string, label: string}} PublicModelFamily */
/**
 * @typedef {{
 *   identifier: string,
 *   publisher: string,
 *   family: string,
 *   version: string,
 *   operations: string[],
 *   media_inputs: string[],
 *   capabilities: string[],
 *   provider_offerings: string[],
 * }} PublicExactModelCapability
 */
/**
 * @typedef {{
 *   identifier: string,
 *   provider: string,
 *   model: string,
 *   capabilities: string[],
 *   wire_contract: string,
 *   output_token_limit: number,
 *   reasoning_efforts: string[],
 * }} PublicProviderOffering
 */
/**
 * @typedef {{
 *   providers: number,
 *   model_publishers: number,
 *   model_families: number,
 *   exact_models: number,
 *   provider_offerings: number,
 * }} PublicCapabilityCounts
 */
/**
 * @typedef {{
 *   providers: PublicProviderCapability[],
 *   publishers: PublicModelPublisher[],
 *   families: PublicModelFamily[],
 *   models: PublicExactModelCapability[],
 *   offerings: PublicProviderOffering[],
 *   counts: PublicCapabilityCounts,
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
    "publishers",
    "families",
    "models",
    "offerings",
    "counts",
    "max_prompt_bytes",
    "max_input_audio_bytes",
    "max_request_timeout_seconds",
  ], "catalog");

  const providers = requiredNonemptyArray(catalog.providers, "catalog.providers").map((rawProvider, providerIndex) => {
    const field = `catalog.providers[${providerIndex}]`;
    const provider = requiredRecord(rawProvider, field);
    requireExactKeys(provider, ["identifier", "label"], field);
    return {
      identifier: requiredString(provider.identifier, `${field}.identifier`),
      label: requiredString(provider.label, `${field}.label`),
    };
  });

  const publishers = requiredNonemptyArray(catalog.publishers, "catalog.publishers").map((rawPublisher, publisherIndex) => {
    const field = `catalog.publishers[${publisherIndex}]`;
    const publisher = requiredRecord(rawPublisher, field);
    requireExactKeys(publisher, ["identifier", "label", "model_count"], field);
    return {
      identifier: requiredString(publisher.identifier, `${field}.identifier`),
      label: requiredString(publisher.label, `${field}.label`),
      model_count: requiredPositiveInteger(publisher.model_count, `${field}.model_count`),
    };
  });

  const families = requiredNonemptyArray(catalog.families, "catalog.families").map((rawFamily, familyIndex) => {
    const field = `catalog.families[${familyIndex}]`;
    const family = requiredRecord(rawFamily, field);
    requireExactKeys(family, ["identifier", "publisher", "label"], field);
    return {
      identifier: requiredString(family.identifier, `${field}.identifier`),
      publisher: requiredString(family.publisher, `${field}.publisher`),
      label: requiredString(family.label, `${field}.label`),
    };
  });

  const models = requiredNonemptyArray(catalog.models, "catalog.models").map((rawModel, modelIndex) => {
    const field = `catalog.models[${modelIndex}]`;
    const model = requiredRecord(rawModel, field);
    requireExactKeys(model, [
      "identifier", "publisher", "family", "version", "operations", "media_inputs", "capabilities", "provider_offerings",
    ], field);
    const operations = requiredNonemptyStringArray(model.operations, `${field}.operations`);
    for (const operation of operations) {
      if (operation !== "text" && operation !== "dictation") {
        throw new Error(`public_capabilities_invalid: ${field}.operations value=${operation}`);
      }
    }
    const mediaInputs = requiredStringArray(model.media_inputs, `${field}.media_inputs`);
    for (const mediaInput of mediaInputs) {
      if (mediaInput !== "image" && mediaInput !== "audio") {
        throw new Error(`public_capabilities_invalid: ${field}.media_inputs value=${mediaInput}`);
      }
    }
    return {
      identifier: requiredString(model.identifier, `${field}.identifier`),
      publisher: requiredString(model.publisher, `${field}.publisher`),
      family: requiredString(model.family, `${field}.family`),
      version: requiredString(model.version, `${field}.version`),
      operations,
      media_inputs: mediaInputs,
      capabilities: parseCapabilities(model.capabilities, `${field}.capabilities`),
      provider_offerings: requiredNonemptyStringArray(model.provider_offerings, `${field}.provider_offerings`),
    };
  });

  const offerings = requiredNonemptyArray(catalog.offerings, "catalog.offerings").map((rawOffering, offeringIndex) => {
    const field = `catalog.offerings[${offeringIndex}]`;
    const offering = requiredRecord(rawOffering, field);
    requireExactKeys(offering, [
      "identifier", "provider", "model", "capabilities", "wire_contract", "output_token_limit", "reasoning_efforts",
    ], field);
    return {
      identifier: requiredString(offering.identifier, `${field}.identifier`),
      provider: requiredString(offering.provider, `${field}.provider`),
      model: requiredString(offering.model, `${field}.model`),
      capabilities: parseCapabilities(offering.capabilities, `${field}.capabilities`),
      wire_contract: requiredString(offering.wire_contract, `${field}.wire_contract`, true),
      output_token_limit: requiredNonnegativeInteger(offering.output_token_limit, `${field}.output_token_limit`),
      reasoning_efforts: requiredStringArray(offering.reasoning_efforts, `${field}.reasoning_efforts`),
    };
  });

  const countsRecord = requiredRecord(catalog.counts, "catalog.counts");
  requireExactKeys(countsRecord, [
    "providers", "model_publishers", "model_families", "exact_models", "provider_offerings",
  ], "catalog.counts");
  const counts = {
    providers: requiredPositiveInteger(countsRecord.providers, "catalog.counts.providers"),
    model_publishers: requiredPositiveInteger(countsRecord.model_publishers, "catalog.counts.model_publishers"),
    model_families: requiredPositiveInteger(countsRecord.model_families, "catalog.counts.model_families"),
    exact_models: requiredPositiveInteger(countsRecord.exact_models, "catalog.counts.exact_models"),
    provider_offerings: requiredPositiveInteger(countsRecord.provider_offerings, "catalog.counts.provider_offerings"),
  };

  const providersByIdentifier = uniqueByIdentifier(providers, "catalog.providers");
  const publishersByIdentifier = uniqueByIdentifier(publishers, "catalog.publishers");
  const familiesByIdentifier = uniqueByIdentifier(families, "catalog.families");
  const modelsByIdentifier = uniqueByIdentifier(models, "catalog.models");
  const offeringsByIdentifier = uniqueByIdentifier(offerings, "catalog.offerings");
  for (const family of families) {
    requireReference(publishersByIdentifier, family.publisher, `family=${family.identifier} publisher`);
  }
  const actualModelCounts = new Map(publishers.map((publisher) => [publisher.identifier, 0]));
  const referencedOfferings = new Set();
  for (const model of models) {
    requireReference(publishersByIdentifier, model.publisher, `model=${model.identifier} publisher`);
    const family = requireReference(familiesByIdentifier, model.family, `model=${model.identifier} family`);
    if (family.publisher !== model.publisher) {
      throw new Error(`public_capabilities_invalid: model=${model.identifier} family_publisher_mismatch`);
    }
    actualModelCounts.set(model.publisher, (actualModelCounts.get(model.publisher) ?? 0) + 1);
    for (const offeringIdentifier of model.provider_offerings) {
      const offering = requireReference(offeringsByIdentifier, offeringIdentifier, `model=${model.identifier} offering`);
      if (offering.model !== model.identifier || referencedOfferings.has(offeringIdentifier)) {
        throw new Error(`public_capabilities_invalid: model=${model.identifier} offering=${offeringIdentifier}`);
      }
      referencedOfferings.add(offeringIdentifier);
    }
  }
  for (const publisher of publishers) {
    if (publisher.model_count !== actualModelCounts.get(publisher.identifier)) {
      throw new Error(`public_capabilities_invalid: publisher=${publisher.identifier} model_count=${publisher.model_count}`);
    }
  }
  for (const offering of offerings) {
    requireReference(providersByIdentifier, offering.provider, `offering=${offering.identifier} provider`);
    requireReference(modelsByIdentifier, offering.model, `offering=${offering.identifier} model`);
    if (offering.identifier !== `${offering.provider}:${offering.model}` || !referencedOfferings.has(offering.identifier)) {
      throw new Error(`public_capabilities_invalid: offering=${offering.identifier} route_identity`);
    }
  }
  requireCount(counts.providers, providers.length, "catalog.counts.providers");
  requireCount(counts.model_publishers, publishers.length, "catalog.counts.model_publishers");
  requireCount(counts.model_families, families.length, "catalog.counts.model_families");
  requireCount(counts.exact_models, models.length, "catalog.counts.exact_models");
  requireCount(counts.provider_offerings, offerings.length, "catalog.counts.provider_offerings");

  return {
    providers,
    publishers,
    families,
    models,
    offerings,
    counts,
    max_prompt_bytes: requiredPositiveInteger(catalog.max_prompt_bytes, "catalog.max_prompt_bytes"),
    max_input_audio_bytes: requiredPositiveInteger(catalog.max_input_audio_bytes, "catalog.max_input_audio_bytes"),
    max_request_timeout_seconds: requiredPositiveInteger(
      catalog.max_request_timeout_seconds,
      "catalog.max_request_timeout_seconds",
    ),
  };
}

/**
 * @param {unknown} rawCapabilities
 * @param {string} field
 * @returns {string[]}
 */
function parseCapabilities(rawCapabilities, field) {
  const capabilities = requiredNonemptyStringArray(rawCapabilities, field);
  for (const capability of capabilities) {
    if (!capabilityDefinitionsByIdentifier.has(capability)) {
      throw new Error(`public_capabilities_invalid: ${field} value=${capability}`);
    }
  }
  return capabilities;
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
  const providersByIdentifier = new Map(catalog.providers.map((provider) => [provider.identifier, provider]));
  const publishersByIdentifier = new Map(catalog.publishers.map((publisher) => [publisher.identifier, publisher]));
  const familiesByIdentifier = new Map(catalog.families.map((family) => [family.identifier, family]));
  const offeringsByIdentifier = new Map(catalog.offerings.map((offering) => [offering.identifier, offering]));
  /** @type {Map<string, PublicExactModelCapability[]>} */
  const modelsByPublisher = new Map(catalog.publishers.map((publisher) => [publisher.identifier, []]));
  for (const model of catalog.models) {
    modelsByPublisher.get(model.publisher)?.push(model);
  }
  const selectedPublisher = catalog.publishers[0];
  const selectedModel = modelsByPublisher.get(selectedPublisher.identifier)?.[0];
  if (!selectedModel) {
    throw new Error(`public_capabilities_invalid: publisher=${selectedPublisher.identifier} has no exact models`);
  }
  const selectedOffering = offeringsByIdentifier.get(selectedModel.provider_offerings[0]);
  if (!selectedOffering) {
    throw new Error(`public_capabilities_invalid: model=${selectedModel.identifier} has no provider offering`);
  }
  const selectedProvider = providersByIdentifier.get(selectedOffering.provider);
  if (!selectedProvider) {
    throw new Error(`public_capabilities_invalid: offering=${selectedOffering.identifier} provider`);
  }
  const publisherButtons = catalog.publishers.map((publisher, publisherIndex) => `        <button type="button" class="routing-tree__branch routing-tree__publisher" data-route-publisher="${escapeAttribute(publisher.identifier)}" aria-controls="routing-tree-models-${escapeAttribute(publisher.identifier)}" aria-pressed="${publisherIndex === 0 ? "true" : "false"}" disabled>
          <strong>${escapeHTML(publisher.label)}</strong><small>${countLabel(publisher.model_count, "model")}</small>
        </button>`).join("");
  const modelGroups = catalog.publishers.map((publisher, publisherIndex) => {
    const publisherModels = modelsByPublisher.get(publisher.identifier) ?? [];
    const modelButtons = publisherModels.map((model, modelIndex) => {
      const family = familiesByIdentifier.get(model.family);
      const searchText = [publisher.identifier, publisher.label, model.identifier, model.version, model.family, family?.label ?? "", ...model.operations].join(" ");
      return `<button type="button" class="routing-tree__branch routing-tree__model" data-route-model="${escapeAttribute(model.identifier)}" data-route-model-publisher="${escapeAttribute(model.publisher)}" data-route-family="${escapeAttribute(model.family)}" data-route-operations="${escapeAttribute(model.operations.join(" "))}" data-route-search-text="${escapeAttribute(searchText)}" aria-pressed="${publisherIndex === 0 && modelIndex === 0 ? "true" : "false"}" disabled><code>${escapeHTML(model.identifier)}</code><small>${escapeHTML(family?.label ?? model.family)} · ${escapeHTML(model.operations.join(" + "))}</small></button>`;
    }).join("");
    return `      <section id="routing-tree-models-${escapeAttribute(publisher.identifier)}" class="routing-tree__model-group" data-route-model-group="${escapeAttribute(publisher.identifier)}" aria-label="${escapeAttribute(publisher.label)} exact models"${publisherIndex === 0 ? "" : " hidden"}>
        <p><strong>${escapeHTML(publisher.label)}</strong><span>${countLabel(publisherModels.length, "exact model")}</span></p>
        <div class="routing-tree__branches routing-tree__model-branches" role="group" aria-label="Choose an exact ${escapeAttribute(publisher.label)} model">
          ${modelButtons}
        </div>
      </section>`;
  }).join("");
  const providerGroups = catalog.models.map((model) => {
    const modelOfferings = model.provider_offerings.map((offeringIdentifier) => requireReference(
      offeringsByIdentifier,
      offeringIdentifier,
      `model=${model.identifier} offering`,
    ));
    const providerButtons = modelOfferings.map((offering, offeringIndex) => {
      const provider = providersByIdentifier.get(offering.provider);
      return `<button type="button" class="routing-tree__branch routing-tree__provider" data-route-provider="${escapeAttribute(offering.provider)}" data-route-offering="${escapeAttribute(offering.identifier)}" aria-pressed="${model.identifier === selectedModel.identifier && offeringIndex === 0 ? "true" : "false"}" disabled><strong>${escapeHTML(provider?.label ?? offering.provider)}</strong><small>${escapeHTML(offering.capabilities.join(" · "))}</small></button>`;
    }).join("");
    return `      <section class="routing-tree__provider-group" data-route-provider-group="${escapeAttribute(model.identifier)}" aria-label="Providers offering ${escapeAttribute(model.identifier)}"${model.identifier === selectedModel.identifier ? "" : " hidden"}>
        <p><strong>Provider offerings</strong><span>${modelOfferings.length} route${modelOfferings.length === 1 ? "" : "s"}</span></p>
        <div class="routing-tree__branches routing-tree__provider-branches" role="group" aria-label="Choose a provider for ${escapeAttribute(model.identifier)}">
          ${providerButtons}
        </div>
      </section>`;
  }).join("");
  const familyOptions = catalog.families.map((family) => `<option value="${escapeAttribute(family.identifier)}">${escapeHTML(family.label)}</option>`).join("");
  return `<routing-tree class="routing-tree" data-enhanced="false" aria-label="Interactive LLM routing map">
  <header class="routing-tree__header">
    <h2>One integration. Choose the exact route.</h2>
    <span>${countLabel(catalog.counts.model_publishers, "publisher")} · ${countLabel(catalog.counts.exact_models, "exact model")} · ${countLabel(catalog.counts.provider_offerings, "offering")}</span>
  </header>
  <form class="routing-tree__filters" data-route-picker role="search" aria-label="Find an exact model">
    <input type="search" aria-label="Search exact models" placeholder="Search publisher, family, or model" autocomplete="off" data-route-search disabled>
    <select aria-label="Filter model family" data-route-family-filter disabled><option value="">All families</option>${familyOptions}</select>
    <select aria-label="Filter model operation" data-route-operation-filter disabled><option value="">All operations</option><option value="text">Text</option><option value="dictation">Dictation</option></select>
    <button type="reset" data-route-reset disabled>Reset</button>
  </form>
  <div class="routing-tree__catalog">
    <section class="routing-tree__stage routing-tree__stage--publishers" aria-labelledby="routing-tree-publisher-title">
      <h3 id="routing-tree-publisher-title">Choose a publisher</h3>
      <div class="routing-tree__branches routing-tree__publisher-branches" role="group" aria-label="Model publishers">
${publisherButtons}
      </div>
    </section>
    <section class="routing-tree__stage routing-tree__stage--models" aria-labelledby="routing-tree-model-title">
      <h3 id="routing-tree-model-title">Choose an exact model</h3>
${modelGroups}
      <p class="routing-tree__empty" data-route-empty hidden>No exact models match these filters.</p>
    </section>
  </div>
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
    <article class="routing-tree__node routing-tree__node--selection" data-route-selection-node>
      <span data-route-selected-publisher>${escapeHTML(selectedPublisher.label)}</span>
      <strong><code data-route-selected-model>${escapeHTML(selectedModel.identifier)}</code></strong>
      <small>${escapeHTML(selectedModel.operations.join(" + "))}</small>
    </article>
    <section class="routing-tree__stage routing-tree__stage--providers" aria-labelledby="routing-tree-provider-title">
      <h3 id="routing-tree-provider-title">Choose a provider offering</h3>
${providerGroups}
    </section>
    <footer class="routing-tree__selection">
      <span>Selected route:</span>
      <output aria-live="polite"><code data-route-selected-provider>${escapeHTML(selectedProvider.identifier)}</code><i aria-hidden="true">/</i><code data-route-selected-route-model>${escapeHTML(selectedModel.identifier)}</code></output>
    </footer>
  </div>
</routing-tree>`;
}

/**
 * @param {PublicCapabilityCatalog} catalog
 * @returns {string}
 */
function renderCapabilityCatalog(catalog) {
  const providersByIdentifier = new Map(catalog.providers.map((provider) => [provider.identifier, provider]));
  const publishersByIdentifier = new Map(catalog.publishers.map((publisher) => [publisher.identifier, publisher]));
  const familiesByIdentifier = new Map(catalog.families.map((family) => [family.identifier, family]));
  const offeringsByIdentifier = new Map(catalog.offerings.map((offering) => [offering.identifier, offering]));
  const availableCapabilities = new Set(catalog.models.flatMap((model) => model.capabilities));
  const filters = capabilityDefinitions.filter((definition) => availableCapabilities.has(definition.identifier));
  const filterControls = filters.map((definition) => `<label class="catalog-filter"><input type="checkbox" name="catalog-capability" value="${escapeAttribute(definition.identifier)}" data-catalog-capability><span>${escapeHTML(definition.label)}</span></label>`).join("");
  const rows = catalog.models.map((model) => renderCapabilityRow(
    publishersByIdentifier.get(model.publisher),
    familiesByIdentifier.get(model.family),
    model,
    model.provider_offerings.map((offeringIdentifier) => requireReference(
      offeringsByIdentifier,
      offeringIdentifier,
      `model=${model.identifier} offering`,
    )),
    providersByIdentifier,
  )).join("");
  return `<capability-catalog data-enhanced="false">
  <div class="catalog-summary" aria-label="Catalog summary">
    <p><strong>${catalog.counts.providers}</strong><span>Providers</span></p>
    <p><strong>${catalog.counts.model_publishers}</strong><span>Publishers</span></p>
    <p><strong>${catalog.counts.model_families}</strong><span>Families</span></p>
    <p><strong>${catalog.counts.exact_models}</strong><span>Exact models</span></p>
    <p><strong>${catalog.counts.provider_offerings}</strong><span>Offerings</span></p>
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
        <output aria-live="polite" data-catalog-result-count>${countLabel(catalog.models.length, "model")}</output>
        <button type="reset" data-catalog-reset>Reset</button>
      </div>
    </div>
  </form>
  <div class="catalog-table-wrap" tabindex="0" role="region" aria-label="Exact model provider offering matrix">
    <table class="catalog-table">
      <caption>Provider-independent exact models and every current provider offering.</caption>
      <thead>
        <tr>
          <th scope="col" aria-sort="ascending" data-catalog-sort-header="publisher"><button type="button" class="catalog-sort-button" data-catalog-sort="publisher" data-sort-label="Publisher" disabled>Publisher<span class="catalog-sort-indicator" aria-hidden="true"></span></button></th>
          <th scope="col" data-catalog-sort-header="model"><button type="button" class="catalog-sort-button" data-catalog-sort="model" data-sort-label="Model" disabled>Model<span class="catalog-sort-indicator" aria-hidden="true"></span></button></th>
          <th scope="col" data-catalog-sort-header="capabilities"><button type="button" class="catalog-sort-button" data-catalog-sort="capabilities" data-sort-label="Capabilities" disabled>Provider offerings and capabilities<span class="catalog-sort-indicator" aria-hidden="true"></span></button></th>
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
 * @param {PublicModelPublisher | undefined} publisher
 * @param {PublicModelFamily | undefined} family
 * @param {PublicExactModelCapability} model
 * @param {PublicProviderOffering[]} offerings
 * @param {Map<string, PublicProviderCapability>} providersByIdentifier
 * @returns {string}
 */
function renderCapabilityRow(publisher, family, model, offerings, providersByIdentifier) {
  if (!publisher || !family || offerings.length === 0) {
    throw new Error(`public_capabilities_invalid: model=${model.identifier} projection`);
  }
  const definitions = capabilityDefinitions.filter((definition) => model.capabilities.includes(definition.identifier));
  const providerIdentifiers = offerings.map((offering) => offering.provider);
  const searchText = [
    publisher.identifier,
    publisher.label,
    family.identifier,
    family.label,
    model.identifier,
    model.version,
    ...model.operations,
    ...model.media_inputs,
    ...model.capabilities,
    ...definitions.map((definition) => definition.label),
    ...offerings.flatMap((offering) => {
      const provider = providersByIdentifier.get(offering.provider);
      return [
        offering.identifier,
        offering.provider,
        provider?.label ?? "",
        offering.wire_contract,
        String(offering.output_token_limit),
        outputLimitLabel(offering),
        ...offering.capabilities,
        ...offering.reasoning_efforts,
      ];
    }),
  ].join(" ");
  const capabilityBadges = definitions.map((definition) => `<button type="button" class="capability-badge ${escapeAttribute(definition.className)}" aria-label="Filter by ${escapeAttribute(definition.label)}" data-catalog-capability-action="${escapeAttribute(definition.identifier)}" disabled>${escapeHTML(definition.label)}</button>`).join("");
  const providerOfferings = offerings.map((offering) => {
    const provider = providersByIdentifier.get(offering.provider);
    const outputLimit = outputLimitLabel(offering);
    const technicalDetails = [
      offering.wire_contract ? `<code>${escapeHTML(offering.wire_contract)}</code>` : "",
      offering.reasoning_efforts.length > 0 ? `<span>Reasoning: ${escapeHTML(offering.reasoning_efforts.join(", "))}</span>` : "",
      outputLimit ? `<span>${escapeHTML(outputLimit)}</span>` : "",
    ].join("");
    return `<li><span class="catalog-offering__provider"><strong>${escapeHTML(provider?.label ?? offering.provider)}</strong><code>${escapeHTML(offering.provider)}</code></span><span class="catalog-technical">${technicalDetails}</span></li>`;
  }).join("");
  return `<tr data-catalog-row data-publisher="${escapeAttribute(publisher.identifier)}" data-provider="${escapeAttribute(providerIdentifiers.join(" "))}" data-model="${escapeAttribute(model.identifier)}" data-capabilities="${escapeAttribute(model.capabilities.join(" "))}" data-capability-count="${definitions.length}" data-catalog-search-text="${escapeAttribute(searchText)}">
          <td class="catalog-publisher"><strong>${escapeHTML(publisher.label)}</strong><code>${escapeHTML(publisher.identifier)}</code></td>
          <td class="catalog-model"><span class="catalog-model__content"><code data-catalog-model-id>${escapeHTML(model.identifier)}</code><small>${escapeHTML(family.label)} · ${escapeHTML(model.version)}</small></span></td>
          <td><div class="catalog-capabilities">
            ${capabilityBadges}
          </div>
          <ul class="catalog-offerings">${providerOfferings}</ul></td>
        </tr>`;
}

/**
 * @param {PublicProviderOffering} offering
 */
function outputLimitLabel(offering) {
  if (!offering.capabilities.includes("text")) {
    return "";
  }
  if (offering.output_token_limit === 0) {
    return "Provider-enforced output";
  }
  return `${offering.output_token_limit} token output`;
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
 * @returns {unknown[]}
 */
function requiredNonemptyArray(value, field) {
  const entries = requiredArray(value, field);
  if (entries.length === 0) {
    throw new Error(`public_capabilities_invalid: ${field} must not be empty`);
  }
  return entries;
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
 * @returns {string[]}
 */
function requiredNonemptyStringArray(value, field) {
  const entries = requiredStringArray(value, field);
  if (entries.length === 0) {
    throw new Error(`public_capabilities_invalid: ${field} must not be empty`);
  }
  return entries;
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
 * @template {{identifier: string}} Entry
 * @param {Entry[]} entries
 * @param {string} field
 * @returns {Map<string, Entry>}
 */
function uniqueByIdentifier(entries, field) {
  const indexedEntries = new Map();
  for (const entry of entries) {
    if (indexedEntries.has(entry.identifier)) {
      throw new Error(`public_capabilities_invalid: ${field} duplicate=${entry.identifier}`);
    }
    indexedEntries.set(entry.identifier, entry);
  }
  return indexedEntries;
}

/**
 * @template Value
 * @param {Map<string, Value>} entries
 * @param {string} identifier
 * @param {string} field
 * @returns {Value}
 */
function requireReference(entries, identifier, field) {
  const entry = entries.get(identifier);
  if (!entry) {
    throw new Error(`public_capabilities_invalid: ${field}=${identifier} dangling_reference`);
  }
  return entry;
}

/**
 * @param {number} declared
 * @param {number} actual
 * @param {string} field
 */
function requireCount(declared, actual, field) {
  if (declared !== actual) {
    throw new Error(`public_capabilities_invalid: ${field}=${declared} actual=${actual}`);
  }
}

/**
 * @param {number} count
 * @param {string} singular
 */
function countLabel(count, singular) {
  return `${count} ${singular}${count === 1 ? "" : "s"}`;
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
