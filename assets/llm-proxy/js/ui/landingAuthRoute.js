// @ts-check

import {
  EVENTS,
  LANDING_AUTHENTICATED_REDIRECT_ATTRIBUTE,
} from "../constants.js?v=20260808b113";

document.addEventListener(EVENTS.AUTHENTICATED, replaceAuthenticatedLanding, { once: true });

/**
 * @param {Event} event
 * @returns {void}
 */
function replaceAuthenticatedLanding(event) {
  const authSurface = event.target;
  if (!(authSurface instanceof HTMLElement)) {
    throw new Error("llm_proxy_landing_auth_surface_invalid");
  }
  const redirectURL = authSurface.getAttribute(LANDING_AUTHENTICATED_REDIRECT_ATTRIBUTE);
  if (!redirectURL) {
    throw new Error("llm_proxy_landing_authenticated_redirect_missing");
  }
  window.location.replace(redirectURL);
}
