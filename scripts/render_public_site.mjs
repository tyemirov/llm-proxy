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
  { identifier: "text", label: "Text generation", routeLabel: "Text", className: "capability-badge--primary" },
  { identifier: "dictation", label: "Dictation", routeLabel: "Dictation", className: "capability-badge--info" },
  { identifier: "video_generation", label: "Video generation", routeLabel: "Video", className: "capability-badge--info" },
  { identifier: "image_input", label: "Image input", routeLabel: "Image", className: "capability-badge--info" },
  { identifier: "audio_input", label: "Audio message input", routeLabel: "Audio", className: "capability-badge--info" },
  { identifier: "caller_tools", label: "Caller tools", routeLabel: "Tools", className: "capability-badge--success" },
  { identifier: "web_search", label: "Web search", routeLabel: "Web search", className: "capability-badge--success" },
  { identifier: "reasoning", label: "Reasoning", routeLabel: "Reasoning", className: "capability-badge--success" },
]);
const capabilityDefinitionsByIdentifier = new Map(
  capabilityDefinitions.map((definition) => [definition.identifier, definition]),
);
const weightAccessDefinitions = Object.freeze([
  { identifier: "proprietary", label: "Proprietary" },
  { identifier: "open_weights", label: "Open weights" },
]);
const weightAccessDefinitionsByIdentifier = new Map(
  weightAccessDefinitions.map((definition) => [definition.identifier, definition]),
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

/** @typedef {{id: string, input_artifacts: string[], output_artifacts: string[]}} PublicModelOperation */
/** @typedef {{identifier: string, label: string, credential_kinds: string[]}} PublicProviderCapability */
/** @typedef {{identifier: string, label: string, model_count: number}} PublicModelPublisher */
/** @typedef {{identifier: string, publisher: string, label: string, weight_access: string}} PublicModelFamily */
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
 *   execution_lifecycle: string,
 *   media_execution_lifecycle: string,
 *   output_token_limit: number,
 *   reasoning_efforts: string[],
 *   controls: PublicCatalogControl[],
 *   limits: PublicCatalogLimit[],
 *   media_limits: PublicCatalogMediaLimit[],
 * }} PublicProviderOffering
 */
/** @typedef {{id: string, kind: string, values: string[], minimum: number | null, maximum: number | null, account_dependent: boolean}} PublicCatalogControl */
/** @typedef {{id: string, value: number | null, unit: string, account_dependent: boolean}} PublicCatalogLimit */
/** @typedef {{id: string, media_type: string, transport: string, status: string, value: number | null, unit: string, scope: string, source: string, last_verified: string}} PublicCatalogMediaLimit */
/** @typedef {{resolution: string, generated_audio: string, input_media: string, output_media: string, duration: string, quantity: string, quality: string, mode: string, api_version: string, avatar_type: string, billing_mode: string, billing_outcome: string}} PublicPriceConditions */
/** @typedef {{component: string, currency: string, rate: number, unit: string, conditions: PublicPriceConditions}} PublicPriceRate */
/** @typedef {{currency: string, amount: number, unit: string}} PublicMinimumCharge */
/** @typedef {{provider: string, model: string, operation: string, available: boolean, rates: PublicPriceRate[], minimum_charge: PublicMinimumCharge | null, source: string, last_verified: string, unavailable_reason: string}} PublicPriceDescriptor */
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
 *   revision: string,
 *   operations: PublicModelOperation[],
 *   providers: PublicProviderCapability[],
 *   publishers: PublicModelPublisher[],
 *   families: PublicModelFamily[],
 *   models: PublicExactModelCapability[],
 *   offerings: PublicProviderOffering[],
 *   prices: PublicPriceDescriptor[],
 *   counts: PublicCapabilityCounts,
 *   max_prompt_bytes: number,
 *   max_input_audio_bytes: number,
 *   max_request_timeout_seconds: number,
 * }} PublicCapabilityCatalog
 */

/**
 * @typedef {{identifier: string, label: string, routeLabel: string, className: string}} CapabilityDefinition
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
    "revision",
    "operations",
    "providers",
    "publishers",
    "families",
    "models",
    "offerings",
    "prices",
    "counts",
    "max_prompt_bytes",
    "max_input_audio_bytes",
    "max_request_timeout_seconds",
  ], "catalog");

  const operations = requiredNonemptyArray(catalog.operations, "catalog.operations").map((rawOperation, operationIndex) => {
    const field = `catalog.operations[${operationIndex}]`;
    const operation = requiredRecord(rawOperation, field);
    requireExactKeys(operation, ["id", "input_artifacts", "output_artifacts"], field);
    const identifier = requiredString(operation.id, `${field}.id`);
    if (identifier !== "text" && identifier !== "dictation" && identifier !== "video_generation") {
      throw new Error(`public_capabilities_invalid: ${field}.id value=${identifier}`);
    }
    return {
      id: identifier,
      input_artifacts: parseArtifactKinds(operation.input_artifacts, `${field}.input_artifacts`),
      output_artifacts: parseArtifactKinds(operation.output_artifacts, `${field}.output_artifacts`),
    };
  });

  const providers = requiredNonemptyArray(catalog.providers, "catalog.providers").map((rawProvider, providerIndex) => {
    const field = `catalog.providers[${providerIndex}]`;
    const provider = requiredRecord(rawProvider, field);
    requireExactKeys(provider, ["identifier", "label", "credential_kinds"], field);
    const credentialKinds = requiredNonemptyStringArray(provider.credential_kinds, `${field}.credential_kinds`);
    if (credentialKinds.length !== 1 || credentialKinds[0] !== "api_key") {
      throw new Error(`public_capabilities_invalid: ${field}.credential_kinds`);
    }
    return {
      identifier: requiredString(provider.identifier, `${field}.identifier`),
      label: requiredString(provider.label, `${field}.label`),
      credential_kinds: credentialKinds,
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
    requireExactKeys(family, ["identifier", "publisher", "label", "weight_access"], field);
    const weightAccess = requiredString(family.weight_access, `${field}.weight_access`);
    if (!weightAccessDefinitionsByIdentifier.has(weightAccess)) {
      throw new Error(`public_capabilities_invalid: ${field}.weight_access value=${weightAccess}`);
    }
    return {
      identifier: requiredString(family.identifier, `${field}.identifier`),
      publisher: requiredString(family.publisher, `${field}.publisher`),
      label: requiredString(family.label, `${field}.label`),
      weight_access: weightAccess,
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
      if (operation !== "text" && operation !== "dictation" && operation !== "video_generation") {
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
    const capabilities = parseCapabilities(offering.capabilities, `${field}.capabilities`);
    const hasMedia = capabilities.includes("image_input") || capabilities.includes("audio_input");
    const offeringKeys = [
      "identifier", "provider", "model", "capabilities", "wire_contract", "execution_lifecycle",
      "output_token_limit", "reasoning_efforts", "controls", "limits", "media_limits",
    ];
    if (hasMedia) {
      offeringKeys.push("media_execution_lifecycle");
    }
    requireExactKeys(offering, offeringKeys, field);
    return {
      identifier: requiredString(offering.identifier, `${field}.identifier`),
      provider: requiredString(offering.provider, `${field}.provider`),
      model: requiredString(offering.model, `${field}.model`),
      capabilities,
      wire_contract: requiredString(offering.wire_contract, `${field}.wire_contract`, true),
      execution_lifecycle: requiredExecutionLifecycle(offering.execution_lifecycle, `${field}.execution_lifecycle`),
      media_execution_lifecycle: hasMedia ? requiredExecutionLifecycle(offering.media_execution_lifecycle, `${field}.media_execution_lifecycle`) : "",
      output_token_limit: requiredNonnegativeInteger(offering.output_token_limit, `${field}.output_token_limit`),
      reasoning_efforts: requiredStringArray(offering.reasoning_efforts, `${field}.reasoning_efforts`),
      controls: requiredArray(offering.controls, `${field}.controls`).map((control, controlIndex) => parseCatalogControl(control, `${field}.controls[${controlIndex}]`)),
      limits: requiredArray(offering.limits, `${field}.limits`).map((limit, limitIndex) => parseCatalogLimit(limit, `${field}.limits[${limitIndex}]`)),
      media_limits: requiredArray(offering.media_limits, `${field}.media_limits`).map((limit, limitIndex) => parseCatalogMediaLimit(limit, `${field}.media_limits[${limitIndex}]`)),
    };
  });

  const prices = requiredNonemptyArray(catalog.prices, "catalog.prices").map((price, priceIndex) => (
    parsePriceDescriptor(price, `catalog.prices[${priceIndex}]`)
  ));

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
  const operationsByIdentifier = uniqueByIdentifier(operations.map((operation) => ({ identifier: operation.id })), "catalog.operations");
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
    for (const operation of model.operations) {
      requireReference(operationsByIdentifier, operation, `model=${model.identifier} operation`);
    }
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
  const priceIdentifiers = new Set();
  for (const price of prices) {
    requireReference(providersByIdentifier, price.provider, `price provider`);
    requireReference(modelsByIdentifier, price.model, `price model`);
    requireReference(operationsByIdentifier, price.operation, `price operation`);
    const priceIdentifier = `${price.provider}:${price.model}:${price.operation}`;
    if (priceIdentifiers.has(priceIdentifier) || !offeringsByIdentifier.has(`${price.provider}:${price.model}`)) {
      throw new Error(`public_capabilities_invalid: price=${priceIdentifier}`);
    }
    priceIdentifiers.add(priceIdentifier);
  }
  requireCount(counts.providers, providers.length, "catalog.counts.providers");
  requireCount(counts.model_publishers, publishers.length, "catalog.counts.model_publishers");
  requireCount(counts.model_families, families.length, "catalog.counts.model_families");
  requireCount(counts.exact_models, models.length, "catalog.counts.exact_models");
  requireCount(counts.provider_offerings, offerings.length, "catalog.counts.provider_offerings");

  return {
    revision: requiredString(catalog.revision, "catalog.revision"),
    operations,
    providers,
    publishers,
    families,
    models,
    offerings,
    prices,
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
 * @param {unknown} rawArtifacts
 * @param {string} field
 */
function parseArtifactKinds(rawArtifacts, field) {
  const artifacts = requiredNonemptyStringArray(rawArtifacts, field);
  for (const artifact of artifacts) {
    if (artifact !== "text" && artifact !== "image" && artifact !== "audio" && artifact !== "video") {
      throw new Error(`public_capabilities_invalid: ${field} value=${artifact}`);
    }
  }
  return artifacts;
}

/**
 * @param {unknown} rawLifecycle
 * @param {string} field
 */
function requiredExecutionLifecycle(rawLifecycle, field) {
  const lifecycle = requiredString(rawLifecycle, field);
  if (lifecycle !== "synchronous_completion" && lifecycle !== "pollable_resource") {
    throw new Error(`public_capabilities_invalid: ${field} value=${lifecycle}`);
  }
  return lifecycle;
}

/**
 * @param {unknown} rawControl
 * @param {string} field
 * @returns {PublicCatalogControl}
 */
function parseCatalogControl(rawControl, field) {
  const control = requiredRecord(rawControl, field);
  requireExactKeys(control, ["id", "kind", "values", "minimum", "maximum", "account_dependent"], field);
  const kind = requiredString(control.kind, `${field}.kind`);
  if (kind !== "enum" && kind !== "integer" && kind !== "boolean") {
    throw new Error(`public_capabilities_invalid: ${field}.kind value=${kind}`);
  }
  return {
    id: requiredString(control.id, `${field}.id`),
    kind,
    values: requiredStringArray(control.values, `${field}.values`),
    minimum: nullableNonnegativeInteger(control.minimum, `${field}.minimum`),
    maximum: nullableNonnegativeInteger(control.maximum, `${field}.maximum`),
    account_dependent: requiredBoolean(control.account_dependent, `${field}.account_dependent`),
  };
}

/**
 * @param {unknown} rawLimit
 * @param {string} field
 * @returns {PublicCatalogLimit}
 */
function parseCatalogLimit(rawLimit, field) {
  const limit = requiredRecord(rawLimit, field);
  requireExactKeys(limit, ["id", "value", "unit", "account_dependent"], field);
  return {
    id: requiredString(limit.id, `${field}.id`),
    value: nullableNonnegativeInteger(limit.value, `${field}.value`),
    unit: requiredString(limit.unit, `${field}.unit`),
    account_dependent: requiredBoolean(limit.account_dependent, `${field}.account_dependent`),
  };
}

/**
 * @param {unknown} rawLimit
 * @param {string} field
 * @returns {PublicCatalogMediaLimit}
 */
function parseCatalogMediaLimit(rawLimit, field) {
  const limit = requiredRecord(rawLimit, field);
  requireExactKeys(limit, [
    "id", "media_type", "transport", "status", "value", "unit", "scope", "source", "last_verified",
  ], field);
  const mediaType = requiredString(limit.media_type, `${field}.media_type`);
  const transport = requiredString(limit.transport, `${field}.transport`);
  const status = requiredString(limit.status, `${field}.status`);
  const unit = requiredString(limit.unit, `${field}.unit`);
  const scope = requiredString(limit.scope, `${field}.scope`);
  const value = nullableNonnegativeInteger(limit.value, `${field}.value`);
  if (!new Set(["all", "image", "audio"]).has(mediaType)
      || !new Set(["any", "inline", "file"]).has(transport)
      || !new Set(["bounded", "unbounded", "unknown"]).has(status)
      || !new Set(["bytes", "files"]).has(unit)
      || !new Set(["attachment", "attachment_encoded_bytes", "request", "request_encoded_bytes"]).has(scope)
      || (status === "bounded" ? value === null || value === 0 : value !== null)) {
    throw new Error(`public_capabilities_invalid: ${field} media limit`);
  }
  const source = requiredString(limit.source, `${field}.source`);
  try {
    if (new URL(source).protocol !== "https:") {
      throw new Error("source protocol");
    }
  } catch {
    throw new Error(`public_capabilities_invalid: ${field}.source`);
  }
  const lastVerified = requiredString(limit.last_verified, `${field}.last_verified`);
  if (!/^\d{4}-\d{2}-\d{2}$/u.test(lastVerified)) {
    throw new Error(`public_capabilities_invalid: ${field}.last_verified`);
  }
  return {
    id: requiredString(limit.id, `${field}.id`),
    media_type: mediaType,
    transport,
    status,
    value,
    unit,
    scope,
    source,
    last_verified: lastVerified,
  };
}

/**
 * @param {unknown} rawPrice
 * @param {string} field
 * @returns {PublicPriceDescriptor}
 */
function parsePriceDescriptor(rawPrice, field) {
  const price = requiredRecord(rawPrice, field);
  requireExactKeys(price, [
    "provider", "model", "operation", "available", "rates", "minimum_charge", "source", "last_verified", "unavailable_reason",
  ], field);
  const rates = requiredArray(price.rates, `${field}.rates`).map((rawRate, rateIndex) => {
    const rateField = `${field}.rates[${rateIndex}]`;
    const rate = requiredRecord(rawRate, rateField);
    requireExactKeys(rate, ["component", "currency", "rate", "unit", "conditions"], rateField);
    return {
      component: requiredString(rate.component, `${rateField}.component`),
      currency: requiredString(rate.currency, `${rateField}.currency`),
      rate: requiredNonnegativeNumber(rate.rate, `${rateField}.rate`),
      unit: requiredString(rate.unit, `${rateField}.unit`),
      conditions: parsePriceConditions(rate.conditions, `${rateField}.conditions`),
    };
  });
  let minimumCharge = null;
  if (price.minimum_charge !== null) {
    const minimum = requiredRecord(price.minimum_charge, `${field}.minimum_charge`);
    requireExactKeys(minimum, ["currency", "amount", "unit"], `${field}.minimum_charge`);
    minimumCharge = {
      currency: requiredString(minimum.currency, `${field}.minimum_charge.currency`),
      amount: requiredNonnegativeNumber(minimum.amount, `${field}.minimum_charge.amount`),
      unit: requiredString(minimum.unit, `${field}.minimum_charge.unit`),
    };
  }
  return {
    provider: requiredString(price.provider, `${field}.provider`),
    model: requiredString(price.model, `${field}.model`),
    operation: requiredString(price.operation, `${field}.operation`),
    available: requiredBoolean(price.available, `${field}.available`),
    rates,
    minimum_charge: minimumCharge,
    source: requiredString(price.source, `${field}.source`),
    last_verified: requiredString(price.last_verified, `${field}.last_verified`),
    unavailable_reason: requiredString(price.unavailable_reason, `${field}.unavailable_reason`, true),
  };
}

/**
 * @param {unknown} rawConditions
 * @param {string} field
 * @returns {PublicPriceConditions}
 */
function parsePriceConditions(rawConditions, field) {
  const conditions = requiredRecord(rawConditions, field);
  const keys = [
    "resolution", "generated_audio", "input_media", "output_media", "duration", "quantity",
    "quality", "mode", "api_version", "avatar_type", "billing_mode", "billing_outcome",
  ];
  requireExactKeys(conditions, keys, field);
  return /** @type {PublicPriceConditions} */ (Object.fromEntries(
    keys.map((key) => [key, requiredString(conditions[key], `${field}.${key}`, true)]),
  ));
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
  const offeringsByIdentifier = new Map(catalog.offerings.map((offering) => [offering.identifier, offering]));
  /** @type {Map<string, PublicExactModelCapability[]>} */
  const modelsByFamily = new Map(catalog.families.map((family) => [family.identifier, []]));
  for (const model of catalog.models) {
    modelsByFamily.get(model.family)?.push(model);
  }
  const defaultWeightAccess = "proprietary";
  const defaultCapability = "text";
  /** @type {Map<string, PublicProviderOffering[]>} */
  const defaultOfferingsByModel = new Map(catalog.models.map((model) => [
    model.identifier,
    model.provider_offerings.map((offeringIdentifier) => requireReference(
      offeringsByIdentifier,
      offeringIdentifier,
      `model=${model.identifier} offering`,
    )).filter((offering) => offering.capabilities.includes(defaultCapability)),
  ]));
  const selectedFamily = catalog.families.find((family) => (
    family.weight_access === defaultWeightAccess
    && (modelsByFamily.get(family.identifier) ?? []).some((model) => (defaultOfferingsByModel.get(model.identifier)?.length ?? 0) > 0)
  ));
  if (!selectedFamily) {
    throw new Error(`public_capabilities_invalid: weight_access=${defaultWeightAccess} capability=${defaultCapability} has no model family`);
  }
  const selectedModel = modelsByFamily.get(selectedFamily.identifier)?.find(
    (model) => (defaultOfferingsByModel.get(model.identifier)?.length ?? 0) > 0,
  );
  if (!selectedModel) {
    throw new Error(`public_capabilities_invalid: family=${selectedFamily.identifier} has no exact models`);
  }
  const selectedOffering = defaultOfferingsByModel.get(selectedModel.identifier)?.[0];
  if (!selectedOffering) {
    throw new Error(`public_capabilities_invalid: model=${selectedModel.identifier} has no provider offering`);
  }
  const selectedProvider = providersByIdentifier.get(selectedOffering.provider);
  if (!selectedProvider) {
    throw new Error(`public_capabilities_invalid: offering=${selectedOffering.identifier} provider`);
  }
  const availableCapabilities = new Set(catalog.offerings.flatMap((offering) => offering.capabilities));
  const accessButtons = weightAccessDefinitions.map((definition) => (
    `<button type="button" class="routing-tree__filter" data-route-weight-access="${escapeAttribute(definition.identifier)}" aria-pressed="${definition.identifier === defaultWeightAccess ? "true" : "false"}" disabled>${escapeHTML(definition.label)}</button>`
  )).join("");
  const capabilityButtons = capabilityDefinitions.filter((definition) => availableCapabilities.has(definition.identifier)).map((definition) => (
    `<button type="button" class="routing-tree__filter" data-route-capability="${escapeAttribute(definition.identifier)}" aria-label="${escapeAttribute(definition.label)}" title="${escapeAttribute(definition.label)}" aria-pressed="${definition.identifier === defaultCapability ? "true" : "false"}" disabled>${escapeHTML(definition.routeLabel)}</button>`
  )).join("");
  const familyButtons = catalog.families.map((family) => {
    const familyModels = modelsByFamily.get(family.identifier) ?? [];
    const matchingModels = familyModels.filter((model) => (defaultOfferingsByModel.get(model.identifier)?.length ?? 0) > 0);
    const visible = family.weight_access === defaultWeightAccess && matchingModels.length > 0;
    return `        <button type="button" class="routing-tree__branch routing-tree__family" data-route-family="${escapeAttribute(family.identifier)}" data-route-family-weight-access="${escapeAttribute(family.weight_access)}" aria-controls="routing-tree-models-${escapeAttribute(family.identifier)}" aria-pressed="${family.identifier === selectedFamily.identifier ? "true" : "false"}"${visible ? "" : " hidden"} disabled>
          <strong>${escapeHTML(family.label)}</strong><small data-route-family-model-count>${countLabel(matchingModels.length, "model")}</small>
        </button>`;
  }).join("");
  const modelGroups = catalog.families.map((family) => {
    const familyModels = modelsByFamily.get(family.identifier) ?? [];
    const matchingModels = familyModels.filter((model) => (defaultOfferingsByModel.get(model.identifier)?.length ?? 0) > 0);
    const modelButtons = familyModels.map((model) => {
      const visible = family.weight_access === defaultWeightAccess && (defaultOfferingsByModel.get(model.identifier)?.length ?? 0) > 0;
      return `<button type="button" class="routing-tree__branch routing-tree__model" data-route-model="${escapeAttribute(model.identifier)}" data-route-model-family="${escapeAttribute(model.family)}" aria-pressed="${model.identifier === selectedModel.identifier ? "true" : "false"}"${visible ? "" : " hidden"} disabled><code>${escapeHTML(model.identifier)}</code><small>${escapeHTML(model.operations.join(" + "))}</small></button>`;
    }).join("");
    return `      <section id="routing-tree-models-${escapeAttribute(family.identifier)}" class="routing-tree__model-group" data-route-model-group="${escapeAttribute(family.identifier)}" aria-label="${escapeAttribute(family.label)} exact models"${family.identifier === selectedFamily.identifier ? "" : " hidden"}>
        <p><strong>${escapeHTML(family.label)}</strong><span data-route-model-count>${countLabel(matchingModels.length, "exact model")}</span></p>
        <div class="routing-tree__branches routing-tree__model-branches" role="group" aria-label="Choose an exact ${escapeAttribute(family.label)} model">
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
    const matchingOfferings = defaultOfferingsByModel.get(model.identifier) ?? [];
    const providerButtons = modelOfferings.map((offering) => {
      const provider = providersByIdentifier.get(offering.provider);
      const visible = offering.capabilities.includes(defaultCapability);
      return `<button type="button" class="routing-tree__branch routing-tree__provider" data-route-provider="${escapeAttribute(offering.provider)}" data-route-offering="${escapeAttribute(offering.identifier)}" data-route-provider-capabilities="${escapeAttribute(offering.capabilities.join(" "))}" aria-pressed="${offering.identifier === selectedOffering.identifier ? "true" : "false"}"${visible ? "" : " hidden"} disabled><strong>${escapeHTML(provider?.label ?? offering.provider)}</strong><small>${escapeHTML(offering.capabilities.join(" · "))}</small></button>`;
    }).join("");
    return `      <section class="routing-tree__provider-group" data-route-provider-group="${escapeAttribute(model.identifier)}" aria-label="Providers offering ${escapeAttribute(model.identifier)}"${model.identifier === selectedModel.identifier ? "" : " hidden"}>
        <p><strong>Provider offerings</strong><span data-route-provider-count>${matchingOfferings.length} route${matchingOfferings.length === 1 ? "" : "s"}</span></p>
        <div class="routing-tree__branches routing-tree__provider-branches" role="group" aria-label="Choose a provider for ${escapeAttribute(model.identifier)}">
          ${providerButtons}
        </div>
      </section>`;
  }).join("");
  const defaultFamilies = catalog.families.filter((family) => family.weight_access === defaultWeightAccess && (
    modelsByFamily.get(family.identifier) ?? []
  ).some((model) => (defaultOfferingsByModel.get(model.identifier)?.length ?? 0) > 0));
  const defaultModels = catalog.models.filter((model) => (
    catalog.families.find((family) => family.identifier === model.family)?.weight_access === defaultWeightAccess
    && (defaultOfferingsByModel.get(model.identifier)?.length ?? 0) > 0
  ));
  const defaultOfferings = defaultModels.flatMap((model) => defaultOfferingsByModel.get(model.identifier) ?? []);
  return `<routing-tree class="routing-tree" data-enhanced="false" aria-label="Interactive LLM routing map">
  <header class="routing-tree__header">
    <h2>One integration. Choose the exact route.</h2>
    <div class="routing-tree__filters" aria-label="Route filters">
      <div class="routing-tree__filter-group" role="group" aria-label="Choose one or both weight access types">${accessButtons}</div>
      <span class="routing-tree__filter-divider" aria-hidden="true"></span>
      <div class="routing-tree__filter-group" role="group" aria-label="Choose one capability">${capabilityButtons}</div>
    </div>
    <output class="routing-tree__counts" aria-live="polite" data-route-counts>${countLabel(defaultFamilies.length, "family", "families")} · ${countLabel(defaultModels.length, "exact model")} · ${countLabel(defaultOfferings.length, "offering")}</output>
  </header>
  <div class="routing-tree__map" data-route-map>
    <canvas class="routing-tree__connectors" data-route-canvas aria-hidden="true"></canvas>
    <article class="routing-tree__node routing-tree__node--product" data-route-product>
      <strong>Your product</strong>
      <span>HTTP · Go · Python · CLI</span>
    </article>
    <article class="routing-tree__node routing-tree__node--proxy" data-route-proxy>
      <strong>LLM Proxy</strong>
      <span>Authenticate · validate · route</span>
    </article>
    <section class="routing-tree__stage routing-tree__stage--families" data-route-stage aria-labelledby="routing-tree-family-title">
      <h3 id="routing-tree-family-title">Choose a model family</h3>
      <div class="routing-tree__branches routing-tree__family-branches" role="group" aria-label="Model families">
${familyButtons}
      </div>
    </section>
    <section class="routing-tree__stage routing-tree__stage--models" data-route-stage aria-labelledby="routing-tree-model-title">
      <h3 id="routing-tree-model-title">Choose an exact model</h3>
${modelGroups}
    </section>
    <section class="routing-tree__stage routing-tree__stage--providers" data-route-stage aria-labelledby="routing-tree-provider-title">
      <h3 id="routing-tree-provider-title">Choose a provider offering</h3>
${providerGroups}
    </section>
    <p class="routing-tree__empty" data-route-empty hidden>No routes match these filters.</p>
    <footer class="routing-tree__selection" data-route-selection>
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
 * @param {unknown} value
 * @param {string} field
 */
function nullableNonnegativeInteger(value, field) {
  if (value === null) {
    return null;
  }
  return requiredNonnegativeInteger(value, field);
}

/**
 * @param {unknown} value
 * @param {string} field
 */
function requiredNonnegativeNumber(value, field) {
  if (typeof value !== "number" || !Number.isFinite(value) || value < 0) {
    throw new Error(`public_capabilities_invalid: ${field} must be a nonnegative number`);
  }
  return value;
}

/**
 * @param {unknown} value
 * @param {string} field
 */
function requiredBoolean(value, field) {
  if (typeof value !== "boolean") {
    throw new Error(`public_capabilities_invalid: ${field} must be a boolean`);
  }
  return value;
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
 * @param {string} [plural]
 */
function countLabel(count, singular, plural = `${singular}s`) {
  return `${count} ${count === 1 ? singular : plural}`;
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
