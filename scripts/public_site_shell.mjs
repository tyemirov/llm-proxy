// @ts-check

import {
  APPLICATION_PATH,
  LANDING_AUTHENTICATED_REDIRECT_ATTRIBUTE,
} from "../site/assets/llm-proxy/js/constants.js";

export const MPR_UI_CSS_URL = "https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui.css";
export const MPR_UI_CONFIG_URL = "https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui-config.js";
export const MPR_UI_BUNDLE_URL = "https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui.js";
export const GOOGLE_IDENTITY_URL = "https://accounts.google.com/gsi/client";
export const JS_YAML_URL = "https://cdn.jsdelivr.net/npm/js-yaml@4.3.0/dist/js-yaml.min.js";
export const LOOPAWARE_PIXEL_URL = "https://loopaware.mprlab.com/pixel.js?site_id=543d2796-d616-4080-99e7-0720ae438440";
export const PUBLIC_FOOTER_COMPACT_MAX_HEIGHT = 56;

const PUBLIC_HEADER_LINKS = Object.freeze({
  alignment: "right",
  links: Object.freeze([
    Object.freeze({ label: "Capabilities", href: "/#capabilities" }),
    Object.freeze({ label: "Models", href: "/#models" }),
    Object.freeze({ label: "API", href: "/docs/" }),
    Object.freeze({ label: "Resources", href: "/resources/" }),
  ]),
});
const PUBLIC_FOOTER_LINKS = Object.freeze({
  alignment: "left",
  links: Object.freeze([
    Object.freeze({ label: "Terms", href: "/terms/" }),
    Object.freeze({ label: "Resources", href: "/resources/" }),
    Object.freeze({ label: "GitHub", href: "https://github.com/tyemirov/llm-proxy" }),
  ]),
});
const MPR_PROJECT_LINKS = Object.freeze({
  style: "drop-up",
  text: "Built by Marco Polo Research Lab",
  links: Object.freeze([
    Object.freeze({ label: "Marco Polo Research Lab", url: "https://mprlab.com" }),
    Object.freeze({ label: "Gravity Notes", url: "https://gravity.mprlab.com" }),
    Object.freeze({ label: "LoopAware", url: "https://loopaware.mprlab.com" }),
    Object.freeze({ label: "Allergy Wheel", url: "https://allergy.mprlab.com" }),
    Object.freeze({ label: "Social Threader", url: "https://threader.mprlab.com" }),
    Object.freeze({ label: "RSVP", url: "https://rsvp.mprlab.com" }),
    Object.freeze({ label: "Countdown Calendar", url: "https://countdown.mprlab.com" }),
    Object.freeze({ label: "LLM Crossword", url: "https://llm-crossword.mprlab.com" }),
    Object.freeze({ label: "Prompt Bubbles", url: "https://prompts.mprlab.com" }),
    Object.freeze({ label: "Wallpapers", url: "https://wallpapers.mprlab.com" }),
  ]),
});

/**
 * @returns {string}
 */
export function renderPublicHeader() {
  return renderPublicHeaderWithAuthenticationRoute(`sign-in-redirect-url="${APPLICATION_PATH}"`);
}

/**
 * @returns {string}
 */
export function renderLandingHeader() {
  return renderPublicHeaderWithAuthenticationRoute(
    `${LANDING_AUTHENTICATED_REDIRECT_ATTRIBUTE}="${APPLICATION_PATH}"`,
  );
}

/**
 * @param {string} authenticationRouteAttribute
 * @returns {string}
 */
function renderPublicHeaderWithAuthenticationRoute(authenticationRouteAttribute) {
  return `    <mpr-header
      class="public-site-header"
      data-config-url="/config-ui.yaml"
      brand-label="LLM Proxy"
      brand-href="/"
      logout-url="/"
      settings="false"
      sign-in-label="Log In"
      ${authenticationRouteAttribute}
      size="small"
      sticky="true"
      auth-transition='{"title":"Opening LLM Proxy","message":"Loading your app."}'
      horizontal-links='${JSON.stringify(PUBLIC_HEADER_LINKS)}'
    >
      <a slot="brand" class="public-site-brand" href="/" aria-label="LLM Proxy home">
        <img src="/assets/llm-proxy/img/llm-proxy-icon.svg" alt="" width="24" height="24" aria-hidden="true">
        <span>LLM Proxy</span>
      </a>
    </mpr-header>`;
}

/**
 * @returns {string}
 */
export function renderPublicFooter() {
  return `    <mpr-footer
      class="public-site-footer"
      size="small"
      sticky="true"
      wrapper-class="public-site-footer-layout"
      privacy-link-label="Privacy"
      privacy-link-href="/privacy/"
      theme-switcher="square"
      horizontal-links='${JSON.stringify(PUBLIC_FOOTER_LINKS)}'
      links-collection='${JSON.stringify(MPR_PROJECT_LINKS)}'
    >
${renderPublicFooterFallback()}
    </mpr-footer>`;
}

/**
 * @returns {string}
 */
function renderPublicFooterFallback() {
  const utilityLinks = [
    Object.freeze({ label: "Privacy", href: "/privacy/" }),
    ...PUBLIC_FOOTER_LINKS.links,
  ];
  return `      <footer class="public-site-footer-fallback" role="contentinfo">
        <nav class="public-site-footer-fallback__nav" aria-label="Footer">
${utilityLinks
    .map((link) => `          <a href="${link.href}">${link.label}</a>`)
    .join("\n")}
        </nav>
        <details class="public-site-footer-fallback__projects">
          <summary>${MPR_PROJECT_LINKS.text}</summary>
          <ul>
${MPR_PROJECT_LINKS.links
    .map((link) => `            <li><a href="${link.url}">${link.label}</a></li>`)
    .join("\n")}
          </ul>
        </details>
      </footer>`;
}

/**
 * Enforces the static public document contract before a generator writes it.
 *
 * @param {string} document
 * @param {string} context
 */
export function assertPublicDocumentShell(document, context) {
  const header = renderPublicHeader();
  const footer = renderPublicFooter();
  const headerStart = document.indexOf(header);
  const mainStart = document.search(/<main\b/i);
  const mainEndStart = mainStart === -1 ? -1 : document.indexOf("</main>", mainStart);
  const mainEnd = mainEndStart === -1 ? -1 : mainEndStart + "</main>".length;
  const footerStart = document.indexOf(footer);

  if (occurrenceCount(document, "<mpr-header") !== 1 || headerStart === -1) {
    throw new Error(`public_document_header_invalid: context=${context}`);
  }
  if (occurrenceCount(document, "<main") !== 1 || mainStart === -1 || mainEnd === -1) {
    throw new Error(`public_document_main_invalid: context=${context}`);
  }
  if (occurrenceCount(document, "<mpr-footer") !== 1 || footerStart === -1) {
    throw new Error(`public_document_footer_invalid: context=${context}`);
  }
  if (!(headerStart < mainStart && mainStart < mainEnd && mainEnd < footerStart)) {
    throw new Error(`public_document_order_invalid: context=${context}`);
  }
  const headerEnd = headerStart + header.length;
  if (document.slice(headerEnd, mainStart).trim() || document.slice(mainEnd, footerStart).trim()) {
    throw new Error(`public_document_content_outside_main: context=${context}`);
  }
  const footerResourcesPattern = /<a\s+href="\/resources\/">Resources<\/a>/;
  if (!footerResourcesPattern.test(footer)) {
    throw new Error(`public_document_footer_resources_invalid: context=${context}`);
  }
  if (/<a\b[^>]*\bhref=["']\/app\/["'][^>]*>/i.test(document)) {
    throw new Error(`public_document_direct_app_link_invalid: context=${context}`);
  }
  assertMPRUIAuthAssets(document, context);
}

/**
 * @param {string} document
 * @param {string} context
 */
export function assertMPRUIAuthAssets(document, context) {
  if (/<script\b[^>]*\bsrc=["'][^"']*\/tauth\.js["'][^>]*><\/script>/i.test(document)) {
    throw new Error(`public_document_tauth_client_invalid: context=${context}`);
  }
  const mprUIConfigScript = `<script src="${MPR_UI_CONFIG_URL}"></script>`;
  if (occurrenceCount(document, mprUIConfigScript) !== 1) {
    throw new Error(`public_document_mpr_ui_config_invalid: context=${context}`);
  }
  const loopAwareScript = `<script defer src="${LOOPAWARE_PIXEL_URL}"></script>`;
  if (occurrenceCount(document, loopAwareScript) !== 1) {
    throw new Error(`public_document_loopaware_pixel_invalid: context=${context}`);
  }
}

/**
 * @param {string} value
 * @param {string} needle
 * @returns {number}
 */
function occurrenceCount(value, needle) {
  return value.split(needle).length - 1;
}

/**
 * @returns {string}
 */
export function renderPublicShellHeadAssets() {
  return `    <link rel="stylesheet" href="${MPR_UI_CSS_URL}">
    <link rel="stylesheet" href="/assets/llm-proxy/public-shell.css">
    <script src="${GOOGLE_IDENTITY_URL}" async defer></script>
    <script src="${JS_YAML_URL}"></script>
    <script src="${MPR_UI_CONFIG_URL}"></script>
    <script
      id="mpr-ui-bundle"
      type="application/json"
      data-mpr-ui-bundle-src="${MPR_UI_BUNDLE_URL}"
    ></script>`;
}
