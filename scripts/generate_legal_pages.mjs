// @ts-check

import { mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname } from "node:path";
import {
  assertPublicDocumentShell,
  LOOPAWARE_PIXEL_URL,
  renderPublicFooter,
  renderPublicHeader,
  renderPublicShellHeadAssets,
} from "./public_site_shell.mjs";

const CHECK_ARGUMENT = "--check";
const PUBLIC_ORIGIN = "https://llm-proxy.mprlab.com";
const EFFECTIVE_DATE = "2026-08-08";
const EFFECTIVE_DATE_TEXT = "August 8, 2026";
const PRODUCT_NAME = "LLM Proxy";
const SERVICE_DESCRIPTION = "LLM Proxy provides authenticated model routing, provider credential management, tenant configuration, and usage visibility.";
const SERVICE_DATA_DESCRIPTION = "account identity, tenant configuration, encrypted provider credentials, client-secret digests, routing settings, request content processed in transit, and content-free usage records";

const LEGAL_PAGES = Object.freeze([
  Object.freeze({
    type: "privacy",
    path: "/privacy/",
    outputPath: "site/privacy/index.html",
    title: "Privacy Policy - LLM Proxy",
    description: "How Marco Polo Research Lab handles account, configuration, request, credential, and usage data for LLM Proxy.",
    introduction: Object.freeze([
      "This Privacy Policy explains how Marco Polo Research Lab LLC handles information when you use LLM Proxy.",
      "LLM Proxy separates browser authentication, tenant configuration, model requests, and content-free usage records so each category is processed only for its stated purpose.",
    ]),
    sections: Object.freeze([
      legalSection("scope", "1. Scope", [
        "This policy applies to the public LLM Proxy website, the authenticated management app, the LLM Proxy API, and related support communications.",
      ]),
      legalSection("information", "2. Information We Handle", [], [
        "Account and authentication data supplied through MPR UI, TAuth, and Google Identity Services, including the name, email address, and profile image associated with the account.",
        "Tenant names, provider and model routing defaults, encrypted upstream provider credentials, and one-way digests of generated LLM Proxy client secrets.",
        "Prompts, message attachments, audio, and other request content while routing a request to the selected model provider and returning its response.",
        "Content-free usage metadata such as tenant, provider, model, operation, status, token counts, timing, and request outcome.",
        "Support communications and the technical records required to secure, operate, and troubleshoot the service.",
      ]),
      legalSection("usage-content-boundary", "3. Usage Record Content Boundary", [
        "LLM Proxy usage records do not store prompts, message attachments, input audio, transcripts, generated responses, raw provider credentials, or raw client secrets.",
      ]),
      legalSection("use", "4. How We Use Information", [], [
        "Authenticate users and protect account access.",
        "Route requests, apply tenant configuration, and return provider responses.",
        "Manage provider credentials and generated client access.",
        "Present usage, failure, and reliability information to authorized users.",
        "Secure, troubleshoot, maintain, and improve LLM Proxy.",
        "Respond to support, privacy, and legal requests.",
      ]),
      legalSection("providers", "5. Service Providers and Model Providers", [
        "Authentication is provided through MPR UI and TAuth, with Google Identity Services available for sign-in. Model request content is sent to the provider selected by the authenticated tenant and is subject to that provider's terms and privacy practices.",
        "The public site uses GitHub Pages. Google Analytics and LoopAware receive website interaction and technical telemetry used to understand aggregate traffic and service quality.",
      ]),
      legalSection("browser-storage", "6. Cookies and Browser Storage", [
        "TAuth uses secure HttpOnly session and refresh cookies for authentication. Browser JavaScript cannot read those cookies. The LLM Proxy backend receives and validates the configured session cookie through TAuth's published validator only to authorize protected LLM Proxy resources. Browser storage may retain interface preferences such as the selected theme; raw provider credentials and generated client secrets are not persisted in browser storage by the app.",
      ]),
      legalSection("sharing", "7. Sharing", [
        "We do not sell personal information. We disclose information to authentication, hosting, analytics, support, and selected model providers only as needed to provide and secure LLM Proxy, or when required by law or needed to protect rights and safety.",
      ]),
      legalSection("retention", "8. Retention", [
        "Account, tenant configuration, credential, and usage records are retained while needed to operate the service, secure accounts, meet legal obligations, and resolve disputes. Request content is processed in transit and is excluded from the LLM Proxy usage record.",
      ]),
      legalSection("security", "9. Security", [
        "We use access controls, transport security, encrypted storage for provider credentials, one-way secret digests, and content-free usage records to protect information. No system can guarantee absolute security.",
      ]),
      legalSection("choices", "10. Your Choices", [
        "You may request access, correction, export, or deletion of personal information, subject to applicable law and records we must retain. Contact support@mprlab.com to make a request or to disconnect an account.",
      ]),
      legalSection("changes", "11. Changes to This Policy", [
        "We may update this policy as LLM Proxy or applicable requirements change. The effective and last-updated dates on this page identify the current version.",
      ]),
      legalSection("contact", "12. Contact", [
        "For privacy questions or data requests, email support@mprlab.com. For legal notices, email legal@mprlab.com. Marco Polo Research Lab LLC can also be reached through https://mprlab.com.",
      ]),
    ]),
  }),
  Object.freeze({
    type: "terms",
    path: "/terms/",
    outputPath: "site/terms/index.html",
    title: "Terms of Service - LLM Proxy",
    description: "Terms governing account access, provider credentials, model requests, and use of LLM Proxy.",
    introduction: Object.freeze([
      "These Terms of Service form an agreement between you and Marco Polo Research Lab LLC for access to and use of LLM Proxy.",
      "By using LLM Proxy, you agree to these Terms and the current Privacy Policy.",
    ]),
    sections: Object.freeze([
      legalSection("eligibility", "1. Eligibility and Account Use", [], [
        "You must be legally able to enter into contracts to use LLM Proxy.",
        "If you use LLM Proxy for an organization, you confirm that you have authority to bind that organization.",
        "You are responsible for account access and activity performed through your account and generated client secrets.",
      ]),
      legalSection("service", "2. Service", [
        SERVICE_DESCRIPTION,
        "Provider availability, model behavior, response quality, limits, and latency depend in part on third-party services and the configuration selected for the tenant.",
      ]),
      legalSection("credentials", "3. Credentials and Third-Party Services", [
        "You are responsible for supplying authorized provider credentials, protecting generated client secrets, and complying with the terms of each selected model provider. LLM Proxy may transmit requests and credentials to those providers only as needed to perform the requested operation.",
      ]),
      legalSection("acceptable-use", "4. Acceptable Use", [], [
        "Use LLM Proxy only for lawful purposes.",
        "Do not attempt unauthorized access, interfere with service availability or security, distribute malicious code, or evade provider or service limits.",
        "Do not submit content or direct processing that violates applicable law, third-party rights, or provider terms.",
      ]),
      legalSection("content", "5. Request Content and Outputs", [
        "You retain rights you hold in request content. You grant us the limited right to process that content to route the request, return the response, secure the service, and troubleshoot the requested operation.",
        "Model outputs may be incomplete, inaccurate, or unsuitable for a particular purpose. You are responsible for reviewing outputs before relying on or publishing them.",
      ]),
      legalSection("availability", "6. Availability and Changes", [
        "We may update, suspend, limit, or discontinue features to maintain security, reliability, provider compatibility, or legal compliance. We do not guarantee uninterrupted availability of LLM Proxy or any third-party provider or model.",
      ]),
      legalSection("intellectual-property", "7. Intellectual Property", [
        "LLM Proxy software, design, documentation, and related materials are owned by Marco Polo Research Lab LLC or its licensors and are protected by applicable intellectual property laws.",
      ]),
      legalSection("disclaimers", "8. Disclaimers", [
        "LLM Proxy is provided as is and as available without warranties of any kind to the maximum extent permitted by law. Model responses are generated by third-party services and are not professional, legal, medical, or financial advice.",
      ]),
      legalSection("liability", "9. Limitation of Liability", [
        "To the maximum extent permitted by law, Marco Polo Research Lab LLC is not liable for indirect, incidental, special, consequential, exemplary, or punitive damages, or for lost profits, revenue, data, or goodwill arising from LLM Proxy.",
      ]),
      legalSection("termination", "10. Suspension and Termination", [
        "We may suspend or terminate access when use violates these Terms, creates legal or security risk, or threatens LLM Proxy or another user. You may stop using LLM Proxy at any time.",
      ]),
      legalSection("changes", "11. Changes to These Terms", [
        "We may update these Terms by publishing a revised version with a new effective date. Continued use after the effective date means you accept the revised Terms.",
      ]),
      legalSection("governing-law", "12. Governing Law", [
        "These Terms are governed by the laws of the State of California, without regard to conflict-of-law rules. Except where applicable law requires otherwise, disputes will be resolved in state or federal courts located in Los Angeles County, California.",
      ]),
      legalSection("contact", "13. Contact", [
        "For support, email support@mprlab.com. For legal notices or questions about these Terms, email legal@mprlab.com. Marco Polo Research Lab LLC can also be reached through https://mprlab.com.",
      ]),
    ]),
  }),
]);
const REQUIRED_LEGAL_FRAGMENTS = Object.freeze({
  privacy: Object.freeze([
    "encrypted upstream provider credentials",
    "one-way digests of generated LLM Proxy client secrets",
    "usage records do not store prompts",
    "Google Analytics and LoopAware",
  ]),
  terms: Object.freeze([
    "responsible for supplying authorized provider credentials",
    "Model outputs may be incomplete",
    "third-party services and the configuration selected for the tenant",
  ]),
});

const unexpectedArguments = process.argv.slice(2).filter((argument) => argument !== CHECK_ARGUMENT);
if (unexpectedArguments.length > 0) {
  throw new Error(`legal_pages_unknown_argument: ${unexpectedArguments.join(",")}`);
}

validateLegalPageCollection();
for (const page of LEGAL_PAGES) {
  validateLegalPageSource(page);
  const rendered = renderLegalPage(page);
  if (process.argv.includes(CHECK_ARGUMENT)) {
    const committed = await readFile(page.outputPath, "utf8").catch(() => "");
    if (committed !== rendered) {
      throw new Error(`legal_page_out_of_date: run node scripts/generate_legal_pages.mjs`);
    }
    console.log(`verified ${page.outputPath}`);
  } else {
    await mkdir(dirname(page.outputPath), { recursive: true });
    await writeFile(page.outputPath, rendered, "utf8");
    console.log(`generated ${page.outputPath}`);
  }
}

/**
 * @param {string} id
 * @param {string} heading
 * @param {string[]} paragraphs
 * @param {string[]} [list]
 */
function legalSection(id, heading, paragraphs, list = []) {
  return Object.freeze({ id, heading, paragraphs: Object.freeze(paragraphs), list: Object.freeze(list) });
}

/**
 * @param {typeof LEGAL_PAGES[number]} page
 * @returns {string}
 */
function renderLegalPage(page) {
  const sections = JSON.stringify(page.sections);
  const document = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>${escapeHTML(page.title)}</title>
    <meta name="description" content="${escapeHTMLAttribute(page.description)}">
    <link rel="canonical" href="${PUBLIC_ORIGIN}${page.path}">
    <meta property="og:title" content="${escapeHTMLAttribute(page.title)}">
    <meta property="og:description" content="${escapeHTMLAttribute(page.description)}">
    <meta property="og:type" content="website">
    <meta property="og:url" content="${PUBLIC_ORIGIN}${page.path}">
    <meta name="twitter:card" content="summary">
    <meta name="theme-color" content="#0f1114">
    <link rel="icon" type="image/svg+xml" href="/assets/llm-proxy/img/favicon.svg">
${renderPublicShellHeadAssets()}
    <link rel="stylesheet" href="/assets/llm-proxy/legal.css">
    <script defer src="/assets/llm-proxy/js/googleAnalytics.js"></script>
    <script defer src="${LOOPAWARE_PIXEL_URL}"></script>
  </head>
  <body class="legal-page">
    <a class="skip-link" href="#main-content">Skip to content</a>
${renderPublicHeader()}
    <main id="main-content" class="legal-main">
      <mpr-legal-document
        type="${page.type}"
        title="${escapeHTMLAttribute(page.title)}"
        product-name="${PRODUCT_NAME}"
        service-description="${escapeHTMLAttribute(SERVICE_DESCRIPTION)}"
        service-data-description="${escapeHTMLAttribute(SERVICE_DATA_DESCRIPTION)}"
        effective-date="${EFFECTIVE_DATE}"
        effective-date-text="${EFFECTIVE_DATE_TEXT}"
        last-updated-date="${EFFECTIVE_DATE}"
        privacy-path="/privacy/"
        terms-path="/terms/"
        sections='${escapeHTMLAttribute(sections)}'
      >
${renderStaticLegalDocument(page)}
      </mpr-legal-document>
    </main>
${renderPublicFooter()}
  </body>
</html>
`;
  assertPublicDocumentShell(document, page.path);
  validateRenderedLegalPage(document, page);
  return document;
}

/**
 * @param {typeof LEGAL_PAGES[number]} page
 */
function validateLegalPageSource(page) {
  const expectedPath = page.type === "privacy" ? "/privacy/" : page.type === "terms" ? "/terms/" : "";
  if (!expectedPath || page.path !== expectedPath || !page.outputPath.endsWith(`${expectedPath}index.html`)) {
    throw new Error(`legal_page_route_invalid: type=${page.type} path=${page.path}`);
  }
  if (!/^\d{4}-\d{2}-\d{2}$/.test(EFFECTIVE_DATE) || !EFFECTIVE_DATE_TEXT.trim()) {
    throw new Error(`legal_page_date_invalid: type=${page.type}`);
  }
  if (!page.title.trim() || !page.description.trim() || page.introduction.length === 0 || page.sections.length === 0) {
    throw new Error(`legal_page_content_missing: type=${page.type}`);
  }
  const sectionIDs = new Set();
  for (const section of page.sections) {
    if (
      !section.id.trim() ||
      !section.heading.trim() ||
      sectionIDs.has(section.id) ||
      (section.paragraphs.length === 0 && section.list.length === 0)
    ) {
      throw new Error(`legal_page_section_invalid: type=${page.type} section=${section.id}`);
    }
    sectionIDs.add(section.id);
  }
  const pageContent = JSON.stringify([page.introduction, page.sections]);
  const requiredFragments = REQUIRED_LEGAL_FRAGMENTS[page.type];
  if (!requiredFragments || requiredFragments.some((fragment) => !pageContent.includes(fragment))) {
    throw new Error(`legal_page_approved_content_missing: type=${page.type}`);
  }
}

function validateLegalPageCollection() {
  const types = new Set(LEGAL_PAGES.map((page) => page.type));
  const paths = new Set(LEGAL_PAGES.map((page) => page.path));
  const outputs = new Set(LEGAL_PAGES.map((page) => page.outputPath));
  if (LEGAL_PAGES.length !== 2 || types.size !== 2 || paths.size !== 2 || outputs.size !== 2) {
    throw new Error("legal_page_collection_invalid");
  }
}

/**
 * @param {string} document
 * @param {typeof LEGAL_PAGES[number]} page
 */
function validateRenderedLegalPage(document, page) {
  const requiredFragments = [
    `<link rel="canonical" href="${PUBLIC_ORIGIN}${page.path}">`,
    `<meta property="og:url" content="${PUBLIC_ORIGIN}${page.path}">`,
    `<mpr-legal-document\n        type="${page.type}"`,
    `effective-date="${EFFECTIVE_DATE}"`,
    `last-updated-date="${EFFECTIVE_DATE}"`,
    `<h1 class="mpr-legal-document__title">${escapeHTML(page.title)}</h1>`,
    'href="/privacy/"',
    'href="/terms/"',
  ];
  if (requiredFragments.some((fragment) => !document.includes(fragment)) || document.includes('href="/tos')) {
    throw new Error(`legal_page_render_invalid: type=${page.type}`);
  }
}

/**
 * @param {typeof LEGAL_PAGES[number]} page
 * @returns {string}
 */
function renderStaticLegalDocument(page) {
  const introduction = page.introduction
    .map((paragraph) => `        <p class="mpr-legal-document__intro">${escapeHTML(paragraph)}</p>`)
    .join("\n");
  const sections = page.sections.map((section) => {
    const paragraphs = section.paragraphs
      .map((paragraph) => `            <p class="mpr-legal-document__paragraph">${escapeHTML(paragraph)}</p>`)
      .join("\n");
    const list = section.list.length > 0
      ? `\n            <ul class="mpr-legal-document__list">\n${section.list
        .map((item) => `              <li class="mpr-legal-document__list-item">${escapeHTML(item)}</li>`)
        .join("\n")}\n            </ul>`
      : "";
    return `          <section id="${section.id}" class="mpr-legal-document__section">
            <h2 class="mpr-legal-document__heading">${escapeHTML(section.heading)}</h2>
${paragraphs}${list}
          </section>`;
  }).join("\n");
  return `        <article class="mpr-legal-document__card">
          <h1 class="mpr-legal-document__title">${escapeHTML(page.title)}</h1>
          <p class="mpr-legal-document__meta"><strong>Effective Date:</strong> <time datetime="${EFFECTIVE_DATE}">${EFFECTIVE_DATE_TEXT}</time> · <strong>Last Updated:</strong> <time datetime="${EFFECTIVE_DATE}">${EFFECTIVE_DATE_TEXT}</time></p>
${introduction}
${sections}
        </article>`;
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
function escapeHTMLAttribute(value) {
  return escapeHTML(value).replaceAll("`", "&#96;");
}
