// @ts-check

import { createHash } from "node:crypto";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname } from "node:path";
import { load } from "js-yaml";
import {
  assertPublicDocumentShell,
  LOOPAWARE_PIXEL_URL,
  renderPublicFooter,
  renderPublicHeader,
  renderPublicShellHeadAssets,
} from "./public_site_shell.mjs";

const CONTRACT_PATH = "docs/openapi.yaml";
const OUTPUT_PATH = "site/docs/index.html";
const PUBLIC_ORIGIN = "https://llm-proxy.mprlab.com";
const API_ORIGIN = "https://llm-proxy-api.mprlab.com";
const SOURCE_URL = "https://github.com/tyemirov/llm-proxy/blob/master/docs/openapi.yaml";
const PUBLIC_SCHEMA_PATH = "/openapi.yaml";
const DOWNLOAD_FILENAME = "llm-proxy-openapi.yaml";
const HTTP_METHODS = new Set(["get", "post", "put", "delete", "patch", "head", "options", "trace"]);
const CHECK_ARGUMENT = "--check";

const unexpectedArguments = process.argv.slice(2).filter((argument) => argument !== CHECK_ARGUMENT);
if (unexpectedArguments.length > 0) {
  throw new Error(`openapi_docs_unknown_argument: ${unexpectedArguments.join(",")}`);
}

const contractSource = await readFile(CONTRACT_PATH, "utf8");
const parsedDocument = load(contractSource);
const document = objectValue(parsedDocument, "document");
validateDocument(document);
const sourceDigest = createHash("sha256").update(contractSource).digest("hex");
const renderedDocument = renderDocument(document, sourceDigest, contractSource);

if (process.argv.includes(CHECK_ARGUMENT)) {
  const committedDocument = await readFile(OUTPUT_PATH, "utf8").catch(() => "");
  if (committedDocument !== renderedDocument) {
    throw new Error(`openapi_docs_out_of_date: run node scripts/generate_openapi_docs.mjs`);
  }
  console.log(`verified ${OUTPUT_PATH} from ${CONTRACT_PATH}`);
} else {
  await mkdir(dirname(OUTPUT_PATH), { recursive: true });
  await writeFile(OUTPUT_PATH, renderedDocument, "utf8");
  console.log(`generated ${OUTPUT_PATH} from ${CONTRACT_PATH}`);
}

/**
 * @param {Record<string, unknown>} document
 */
function validateDocument(document) {
  if (document.openapi !== "3.1.0") {
    throw new Error(`openapi_docs_version_invalid: ${String(document.openapi)}`);
  }
  const servers = arrayValue(document.servers, "servers");
  if (servers.length !== 1 || objectValue(servers[0], "servers[0]").url !== API_ORIGIN) {
    throw new Error("openapi_docs_server_invalid");
  }
  const operations = documentOperations(document);
  const operationIDs = operations.map((operation) => operation.operationId);
  if (new Set(operationIDs).size !== operationIDs.length) {
    throw new Error("openapi_docs_duplicate_operation_id");
  }
  for (const operation of operations) {
    if (Object.keys(objectValue(operation.operation.responses, `${operation.operationId}.responses`)).length === 0) {
      throw new Error(`openapi_docs_responses_missing: operation=${operation.operationId}`);
    }
  }
  const v2Operation = operations.find((operation) => operation.operationId === "postV2Messages");
  if (!v2Operation) {
    throw new Error("openapi_docs_v2_operation_missing");
  }
  const v2RequestSchema = operationRequestSchema(document, v2Operation.operation);
  const v2Properties = objectValue(v2RequestSchema.properties, "postV2Messages.properties");
  if (!Object.hasOwn(v2Properties, "reasoning_effort")) {
    throw new Error("openapi_docs_v2_reasoning_effort_missing");
  }
}

/**
 * @param {Record<string, unknown>} document
 * @param {string} sourceDigest
 * @param {string} contractSource
 * @returns {string}
 */
function renderDocument(document, sourceDigest, contractSource) {
  const info = objectValue(document.info, "info");
  const operations = documentOperations(document);
  const operationMarkup = operations.map((operation) => renderOperation(document, operation)).join("\n");
  const renderedDocument = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>${escapeHTML(String(info.title))} | LLM Proxy</title>
    <meta name="description" content="Human-readable API reference derived from the canonical LLM Proxy OpenAPI contract.">
    <link rel="canonical" href="${PUBLIC_ORIGIN}/docs/">
    <meta name="openapi-source-sha256" content="${sourceDigest}">
    <link rel="icon" type="image/svg+xml" href="/assets/llm-proxy/img/favicon.svg">
${renderPublicShellHeadAssets()}
    <script defer src="${LOOPAWARE_PIXEL_URL}"></script>
    <link rel="stylesheet" href="/assets/llm-proxy/styles.css">
    <link rel="stylesheet" href="/assets/llm-proxy/resources.css">
  </head>
  <body class="resource-page api-reference-page" data-openapi-source-sha256="${sourceDigest}">
${renderPublicHeader()}
    <main class="resource-shell resource-article api-reference">
      <section id="openapi-schema" class="resource-hero">
        <p class="eyebrow">Canonical API contract</p>
        <h1>${escapeHTML(String(info.title))}</h1>
        <p class="resource-deck">${escapeHTML(String(info.description))}</p>
        <div class="api-contract-meta" aria-label="Contract provenance">
          <span>OpenAPI ${escapeHTML(String(document.openapi))}</span>
          <span>API server ${escapeHTML(API_ORIGIN)}</span>
          <span>Source SHA-256 <code>${sourceDigest}</code></span>
        </div>
        <div class="resource-actions" aria-label="OpenAPI schema actions">
          <a class="resource-button" href="#openapi-yaml">View YAML</a>
          <a class="resource-button" href="${PUBLIC_SCHEMA_PATH}" download="${DOWNLOAD_FILENAME}">Download YAML</a>
          <a class="resource-link" href="${SOURCE_URL}">View committed source</a>
        </div>
      </section>
      <section id="openapi-yaml" aria-labelledby="openapi-yaml-title">
        <h2 id="openapi-yaml-title">OpenAPI YAML</h2>
        <p>The complete canonical manifest is shown below. The download action publishes these exact source bytes.</p>
        <pre class="api-schema-source" tabindex="0"><code>${escapeHTML(contractSource)}</code></pre>
      </section>
      <section aria-labelledby="operation-index-title">
        <h2 id="operation-index-title">Operations</h2>
        <div class="api-operation-index">
          ${operations
            .map(
              (operation) =>
                `<a href="#operation-${escapeAttribute(operation.operationId)}"><span class="api-method api-method-${operation.method}">${escapeHTML(operation.method.toUpperCase())}</span><code>${escapeHTML(operation.path)}</code><span>${escapeHTML(String(operation.operation.summary || operation.operationId))}</span></a>`,
            )
            .join("\n")}
        </div>
      </section>
      ${operationMarkup}
    </main>
${renderPublicFooter()}
  </body>
</html>
`;
  assertPublicDocumentShell(renderedDocument, `${PUBLIC_ORIGIN}/docs/`);
  return renderedDocument;
}

/**
 * @param {Record<string, unknown>} document
 * @param {{ path: string, method: string, operationId: string, operation: Record<string, unknown> }} item
 * @returns {string}
 */
function renderOperation(document, item) {
  const operation = item.operation;
  const parameters = operationParameters(document, operation);
  const requestSchema = operation.requestBody ? operationRequestSchema(document, operation) : null;
  const requestProperties = requestSchema ? objectValue(requestSchema.properties || {}, `${item.operationId}.properties`) : {};
  const requiredProperties = new Set(requestSchema ? stringArray(requestSchema.required || [], `${item.operationId}.required`) : []);
  const responses = objectValue(operation.responses, `${item.operationId}.responses`);
  const descriptionMarkup = operation.description
    ? `\n        <p>${escapeHTML(String(operation.description))}</p>`
    : "";
  const requiredContentTypeMarkup = operation["x-llm-proxy-required-request-content-type"]
    ? `\n        <p class="api-security"><strong>Required Content-Type:</strong> <code>${escapeHTML(String(operation["x-llm-proxy-required-request-content-type"]))}</code></p>`
    : "";
  return `<section class="api-operation" id="operation-${escapeAttribute(item.operationId)}">
        <header class="api-operation-heading">
          <span class="api-method api-method-${item.method}">${escapeHTML(item.method.toUpperCase())}</span>
          <code>${escapeHTML(item.path)}</code>
          <span class="api-operation-id">${escapeHTML(item.operationId)}</span>
        </header>
        <h2>${escapeHTML(String(operation.summary || item.operationId))}</h2>${descriptionMarkup}
        <p class="api-security"><strong>Authentication:</strong> ${escapeHTML(operationSecurity(document, operation))}</p>${requiredContentTypeMarkup}
        ${renderParameterTable(document, parameters)}
        ${renderRequestFields(document, requestProperties, requiredProperties)}
        ${renderResponses(document, responses)}
      </section>`;
}

/**
 * @param {Record<string, unknown>} document
 * @param {Record<string, unknown>[]} parameters
 * @returns {string}
 */
function renderParameterTable(document, parameters) {
  if (parameters.length === 0) {
    return `<div class="api-contract-block"><h3>Parameters</h3><p>None.</p></div>`;
  }
  return `<div class="api-contract-block">
          <h3>Parameters</h3>
          <div class="resource-table-wrap">
            <table>
              <thead><tr><th>Name</th><th>Location</th><th>Required</th><th>Type</th><th>Description</th></tr></thead>
              <tbody>
                ${parameters
                  .map((parameter) => {
                    const schema = resolveReference(document, objectValue(parameter.schema || {}, "parameter.schema"));
                    return `<tr><td><code>${escapeHTML(String(parameter.name))}</code></td><td>${escapeHTML(String(parameter.in))}</td><td>${parameter.required === true ? "Yes" : "No"}</td><td>${escapeHTML(schemaType(schema))}</td><td>${escapeHTML(String(parameter.description || ""))}</td></tr>`;
                  })
                  .join("\n")}
              </tbody>
            </table>
          </div>
        </div>`;
}

/**
 * @param {Record<string, unknown>} document
 * @param {Record<string, unknown>} properties
 * @param {Set<string>} requiredProperties
 * @returns {string}
 */
function renderRequestFields(document, properties, requiredProperties) {
  const propertyEntries = Object.entries(properties);
  if (propertyEntries.length === 0) {
    return `<div class="api-contract-block"><h3>Request body</h3><p>None.</p></div>`;
  }
  return `<div class="api-contract-block">
          <h3>Request fields</h3>
          <div class="resource-table-wrap">
            <table>
              <thead><tr><th>Field</th><th>Required</th><th>Type</th><th>Description</th></tr></thead>
              <tbody>
                ${propertyEntries
                  .map(([propertyName, rawSchema]) => {
                    const schema = resolveReference(document, objectValue(rawSchema, `property.${propertyName}`));
                    return `<tr><td><code>${escapeHTML(propertyName)}</code></td><td>${requiredProperties.has(propertyName) ? "Yes" : "No"}</td><td>${escapeHTML(schemaType(schema))}</td><td>${escapeHTML(String(schema.description || ""))}</td></tr>`;
                  })
                  .join("\n")}
              </tbody>
            </table>
          </div>
        </div>`;
}

/**
 * @param {Record<string, unknown>} document
 * @param {Record<string, unknown>} responses
 * @returns {string}
 */
function renderResponses(document, responses) {
  return `<div class="api-contract-block">
          <h3>Responses</h3>
          <div class="resource-table-wrap">
            <table>
              <thead><tr><th>Status</th><th>Content types</th><th>Headers</th><th>Description</th></tr></thead>
              <tbody>
                ${Object.entries(responses)
                  .map(([status, rawResponse]) => {
                    const response = resolveReference(document, objectValue(rawResponse, `response.${status}`));
                    const content = objectValue(response.content || {}, `response.${status}.content`);
                    const headers = objectValue(response.headers || {}, `response.${status}.headers`);
                    return `<tr><td><code>${escapeHTML(status)}</code></td><td>${escapeHTML(Object.keys(content).join(", ") || "none")}</td><td>${escapeHTML(Object.keys(headers).join(", ") || "none")}</td><td>${escapeHTML(String(response.description || ""))}</td></tr>`;
                  })
                  .join("\n")}
              </tbody>
            </table>
          </div>
        </div>`;
}

/**
 * @param {Record<string, unknown>} document
 * @returns {{ path: string, method: string, operationId: string, operation: Record<string, unknown> }[]}
 */
function documentOperations(document) {
  const paths = objectValue(document.paths, "paths");
  return Object.entries(paths)
    .flatMap(([path, rawPathItem]) => {
      const pathItem = objectValue(rawPathItem, `paths.${path}`);
      return Object.entries(pathItem)
        .filter(([method]) => HTTP_METHODS.has(method))
        .map(([method, rawOperation]) => {
          const operation = objectValue(rawOperation, `paths.${path}.${method}`);
          const operationId = String(operation.operationId || "");
          if (!operationId) {
            throw new Error(`openapi_docs_operation_id_missing: method=${method} path=${path}`);
          }
          return { path, method, operationId, operation };
        });
    })
    .sort((left, right) => left.path.localeCompare(right.path) || left.method.localeCompare(right.method));
}

/**
 * @param {Record<string, unknown>} document
 * @param {Record<string, unknown>} operation
 * @returns {Record<string, unknown>[]}
 */
function operationParameters(document, operation) {
  return arrayValue(operation.parameters || [], "operation.parameters").map((parameter, parameterIndex) =>
    resolveReference(document, objectValue(parameter, `operation.parameters[${parameterIndex}]`)),
  );
}

/**
 * @param {Record<string, unknown>} document
 * @param {Record<string, unknown>} operation
 * @returns {Record<string, unknown>}
 */
function operationRequestSchema(document, operation) {
  const requestBody = resolveReference(document, objectValue(operation.requestBody, "operation.requestBody"));
  const content = objectValue(requestBody.content, "operation.requestBody.content");
  const firstMediaContract = Object.values(content)[0];
  const mediaContract = objectValue(firstMediaContract, "operation.requestBody.content media");
  return resolveReference(document, objectValue(mediaContract.schema, "operation.requestBody.schema"));
}

/**
 * @param {Record<string, unknown>} document
 * @param {Record<string, unknown>} operation
 * @returns {string}
 */
function operationSecurity(document, operation) {
  const security = arrayValue(operation.security || [], "operation.security");
  if (security.length === 0) {
    return "None";
  }
  const schemes = objectValue(objectValue(document.components, "components").securitySchemes, "components.securitySchemes");
  return security
    .flatMap((rawRequirement) => Object.keys(objectValue(rawRequirement, "security requirement")))
    .map((schemeName) => {
      const scheme = objectValue(schemes[schemeName], `security scheme ${schemeName}`);
      return `${schemeName} (${String(scheme.in)} ${String(scheme.name)})`;
    })
    .join(", ");
}

/**
 * @param {Record<string, unknown>} document
 * @param {Record<string, unknown>} value
 * @returns {Record<string, unknown>}
 */
function resolveReference(document, value) {
  const reference = value.$ref;
  if (reference === undefined) {
    return value;
  }
  if (typeof reference !== "string" || !reference.startsWith("#/")) {
    throw new Error(`openapi_docs_reference_invalid: ${String(reference)}`);
  }
  /** @type {unknown} */
  let resolved = document;
  for (const encodedSegment of reference.slice(2).split("/")) {
    const segment = encodedSegment.replaceAll("~1", "/").replaceAll("~0", "~");
    resolved = objectValue(resolved, `reference ${reference}`)[segment];
  }
  return objectValue(resolved, `reference ${reference}`);
}

/**
 * @param {Record<string, unknown>} schema
 * @returns {string}
 */
function schemaType(schema) {
  const type = String(schema.type || "object");
  if (type === "array") {
    const items = objectValue(schema.items || {}, "array.items");
    const itemReference = typeof items.$ref === "string" ? items.$ref.split("/").at(-1) : items.type;
    return `array<${String(itemReference || "object")}>`;
  }
  return type;
}

/**
 * @param {unknown} value
 * @param {string} context
 * @returns {Record<string, unknown>}
 */
function objectValue(value, context) {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new Error(`openapi_docs_object_required: ${context}`);
  }
  return /** @type {Record<string, unknown>} */ (value);
}

/**
 * @param {unknown} value
 * @param {string} context
 * @returns {unknown[]}
 */
function arrayValue(value, context) {
  if (!Array.isArray(value)) {
    throw new Error(`openapi_docs_array_required: ${context}`);
  }
  return value;
}

/**
 * @param {unknown} value
 * @param {string} context
 * @returns {string[]}
 */
function stringArray(value, context) {
  const values = arrayValue(value, context);
  if (!values.every((item) => typeof item === "string")) {
    throw new Error(`openapi_docs_string_array_required: ${context}`);
  }
  return /** @type {string[]} */ (values);
}

/**
 * @param {string} value
 * @returns {string}
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
 * @returns {string}
 */
function escapeAttribute(value) {
  return escapeHTML(value.replaceAll(" ", "-"));
}
