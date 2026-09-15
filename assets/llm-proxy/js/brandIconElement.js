// @ts-check

import { renderBrandIcon } from "./brandIcons.js?v=20260903f037";

/** Present a catalog logo when Alpine supplies its identity. */
class BrandIconElement extends HTMLElement {
  static observedAttributes = ["kind", "identifier"];

  connectedCallback() {
    this.render();
  }

  attributeChangedCallback() {
    this.render();
  }

  render() {
    const kind = this.getAttribute("kind");
    const identifier = this.getAttribute("identifier");
    if (kind === null || identifier === null) return;
    const markup = renderBrandIcon(kind, identifier);
    this.hidden = markup === "";
    this.innerHTML = markup;
  }
}

customElements.define("brand-icon", BrandIconElement);
