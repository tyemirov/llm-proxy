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
  USER_MENU_ITEM: "mpr-user:menu-item",
  MANAGEMENT_READY: "llm-proxy:management-ready",
});

export const MENU_ACTIONS = Object.freeze({
  OPEN_ADMIN: "open-admin",
  OPEN_SETTINGS: "open-settings",
});

export const DASHBOARD_VIEWS = Object.freeze({
  USAGE: "usage",
  ADMIN: "admin",
});

export const USER_MENU_ITEMS = Object.freeze([
  Object.freeze({
    label: "Settings",
    action: MENU_ACTIONS.OPEN_SETTINGS,
  }),
]);

export const ADMIN_USER_MENU_ITEMS = Object.freeze([
  Object.freeze({
    label: "Admin",
    action: MENU_ACTIONS.OPEN_ADMIN,
  }),
  ...USER_MENU_ITEMS,
]);

export const MPR_UI = Object.freeze({
  CONFIG_URL_ATTRIBUTE: "data-config-url",
  HEADER_ID: "llm-proxy-header",
  ORCHESTRATION_LOADER_MISSING: "llm_proxy_mpr_ui_orchestration_loader_missing",
  USER_SELECTOR: "mpr-user",
  USER_MENU_ITEMS_ATTRIBUTE: "menu-items",
  YAML_LOADER_MISSING: "llm_proxy_yaml_loader_missing",
});

export const COPY = Object.freeze({
  loadingEyebrow: "Session",
  loadingTitle: "Loading key workspace",
  signedOutEyebrow: "Authentication",
  signedOutTitle: "Sign in to manage llm-proxy keys",
  dashboardEyebrow: "Dashboard",
  dashboardTitle: "Usage overview",
  adminDashboardEyebrow: "Admin",
  adminDashboardTitle: "All users",
  refreshUsage: "Refresh",
  refreshAdmin: "Refresh",
  openUsageDashboard: "Usage overview",
  usageRequests: "Requests",
  usageTokens: "Tokens",
  usageSuccessRate: "Success rate",
  usageProviders: "Providers",
  usageRequestTrend: "Requests",
  usageTokenTrend: "Tokens",
  usageByProvider: "Provider usage",
  usageByModel: "Model usage",
  usageEmpty: "No usage recorded",
  adminEmpty: "No managed users",
  adminTenant: "Tenant",
  adminSecret: "Secret",
  adminSecretReady: "Created",
  adminSecretMissing: "Missing",
  adminUpdated: "Updated",
  adminUserFallback: "Unnamed user",
  settingsEyebrow: "Workspace",
  settingsTitle: "Settings",
  closeSettings: "Close",
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
  noDictationModel: "No dictation model",
  systemPrompt: "System prompt",
  saveDefaults: "Save defaults",
  examplesEyebrow: "Usage",
  examplesTitle: "Request examples",
  defaultTextExample: "Default text",
  defaultV2Example: "Default v2",
  defaultDictationExample: "Default dictation",
  providerTextExampleSuffix: " text",
  providerV2ExampleSuffix: " v2",
  providerDictationExampleSuffix: " dictation",
  copyExample: "Copy",
  providersEyebrow: "Providers",
  providersTitle: "Provider settings",
  providerSelector: "Provider",
  providerMissing: "No key saved",
  providerKeySuffix: " API key",
  providerTextModel: "Text model",
  providerSystemPrompt: "System prompt",
  removeProviderKey: "Remove provider key and settings",
  updateProviderKey: "Update key",
  saveProviderKey: "Save key",
  profileLoaded: "Workspace loaded",
  usageRefreshed: "Usage refreshed",
  providerKeySaved: "Provider key saved",
  providerKeyRemoved: "Provider key and settings removed",
  defaultsSaved: "Defaults saved",
  secretGenerated: "Secret created",
  secretRevoked: "Secret revoked",
  secretCopied: "Secret copied",
  exampleCopied: "Example copied",
  copyUnavailable: "Copy unavailable",
  authenticationRequired: "Authentication required",
  requestFailed: "Request failed",
});
