// @ts-check

export const MPR_UI_CSS_URL = "https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui.css";
export const MPR_UI_BUNDLE_URL = "https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui.js";

const PUBLIC_HEADER_LINKS = Object.freeze({
  alignment: "right",
  links: Object.freeze([
    Object.freeze({ label: "Capabilities", href: "/#capabilities" }),
    Object.freeze({ label: "Models", href: "/#models" }),
    Object.freeze({ label: "API", href: "/docs/" }),
    Object.freeze({ label: "Resources", href: "/resources/" }),
    Object.freeze({ label: "Log In", href: "/app/" }),
  ]),
});
const PUBLIC_FOOTER_LINKS = Object.freeze({
  alignment: "right",
  links: Object.freeze([
    Object.freeze({ label: "Log In", href: "/app/" }),
    Object.freeze({ label: "API", href: "/docs/" }),
    Object.freeze({ label: "OpenAPI", href: "/docs/#openapi-schema" }),
    Object.freeze({ label: "Resources", href: "/resources/" }),
    Object.freeze({ label: "GitHub", href: "https://github.com/tyemirov/llm-proxy" }),
  ]),
});

/**
 * @returns {string}
 */
export function renderPublicHeader() {
  return `    <mpr-header
      class="public-site-header"
      brand-label="LLM Proxy"
      brand-href="/"
      settings="false"
      size="small"
      sticky="true"
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
      sticky="false"
      prefix-text="LLM Proxy"
      privacy-link-hidden="true"
      horizontal-links='${JSON.stringify(PUBLIC_FOOTER_LINKS)}'
    ></mpr-footer>`;
}

/**
 * @returns {string}
 */
export function renderPublicShellHeadAssets() {
  return `    <link rel="stylesheet" href="${MPR_UI_CSS_URL}">
    <link rel="stylesheet" href="/assets/llm-proxy/public-shell.css">
    <script defer src="${MPR_UI_BUNDLE_URL}"></script>`;
}
