// @ts-check

export const AUTH_STATES = Object.freeze({
  LOADING: "loading",
  AUTHENTICATED: "authenticated",
  UNAUTHENTICATED: "unauthenticated",
});

export const NOTICE_KINDS = Object.freeze({
  INFO: "info",
  SUCCESS: "success",
  ERROR: "error",
});

export const EVENTS = Object.freeze({
  AUTHENTICATED: "mpr-ui:auth:authenticated",
  UNAUTHENTICATED: "mpr-ui:auth:unauthenticated",
  MANAGEMENT_READY: "llm-proxy:management-ready",
});

export const MPR_UI = Object.freeze({
  BUNDLE_URL: "https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@v3.9.0/mpr-ui.js",
  CONFIG_URL_ATTRIBUTE: "data-config-url",
  CONFIG_URL_PLACEHOLDER: "__LLM_PROXY_CONFIG_URL__",
  CONFIG_LOADER_MISSING: "llm_proxy_mpr_ui_config_loader_missing",
  HEADER_ID: "llm-proxy-header",
  SCRIPT_ID: "llm-proxy-mpr-ui-script",
  YAML_LOADER_MISSING: "llm_proxy_yaml_loader_missing",
});

export const COPY = Object.freeze({
  loadingEyebrow: "Session",
  loadingTitle: "Loading key workspace",
  signedOutEyebrow: "Authentication",
  signedOutTitle: "Sign in to manage llm-proxy keys",
  tenantEyebrow: "Tenant",
  tenantTitle: "Client access",
  generateSecret: "Create key",
  revokeSecret: "Revoke key",
  tenantId: "Tenant ID",
  secretStatus: "Secret",
  secretReady: "Created",
  secretMissing: "Not created",
  generatedSecret: "Generated secret",
  copySecret: "Copy secret",
  defaultsEyebrow: "Defaults",
  defaultsTitle: "Routing defaults",
  textProvider: "Text provider",
  textModel: "Text model",
  dictationProvider: "Dictation provider",
  dictationModel: "Dictation model",
  noDictationDefault: "No dictation default",
  noDictationModel: "No dictation model",
  systemPrompt: "System prompt",
  saveDefaults: "Save defaults",
  usageEyebrow: "Usage",
  usageTitle: "Request example",
  providersEyebrow: "Providers",
  providersTitle: "Provider keys",
  providerMissing: "No key saved",
  providerKeySuffix: " API key",
  removeProviderKey: "Remove key",
  updateProviderKey: "Update key",
  saveProviderKey: "Save key",
  profileLoaded: "Workspace loaded",
  providerKeySaved: "Provider key saved",
  providerKeyRemoved: "Provider key removed",
  defaultsSaved: "Defaults saved",
  secretGenerated: "Secret created",
  secretRevoked: "Secret revoked",
  secretCopied: "Secret copied",
  copyUnavailable: "Copy unavailable",
  authenticationRequired: "Authentication required",
  requestFailed: "Request failed",
});
