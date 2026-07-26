// @ts-check

export const AUTH_STATES = Object.freeze({
  LOADING: "loading",
  AUTHENTICATED: "authenticated",
  UNAUTHENTICATED: "unauthenticated",
  ERROR: "error",
});

export const NOTICE_KINDS = Object.freeze({
  INFO: "info",
  SUCCESS: "success",
  ERROR: "error",
});

export const NOTICE_AUTO_DISMISS_MILLISECONDS = 10_000;

export const EVENTS = Object.freeze({
  AUTHENTICATED: "mpr-ui:auth:authenticated",
  AUTH_STATUS_CHANGE: "mpr-ui:auth:status-change",
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

export const USAGE_INTERVALS = Object.freeze([
  Object.freeze({ id: "all", label: "ALL", bucketUnit: "day" }),
  Object.freeze({ id: "30d", label: "30 days", bucketUnit: "day" }),
  Object.freeze({ id: "7d", label: "7 days", bucketUnit: "day" }),
  Object.freeze({ id: "1d", label: "1 day", bucketUnit: "hour" }),
]);

export const DEFAULT_USAGE_INTERVAL = "30d";
export const USAGE_FAILURE_PAGE_LIMIT = 25;

export const USAGE_ENDPOINT_LABELS = Object.freeze({
  text: "Text",
  v2: "V2",
  dictation: "Dictation",
});

export const USAGE_OUTCOME_LABELS = Object.freeze({
  success: "Success",
  invalid_request: "Invalid request",
  payload_too_large: "Payload too large",
  rate_limited: "Rate limited",
  service_unavailable: "Service unavailable",
  request_timeout: "Request timeout",
  upstream_error: "Upstream error",
});

export const USAGE_STATUS_LABELS = Object.freeze({
  400: "Bad request",
  413: "Payload too large",
  429: "Rate limited",
  502: "Upstream error",
  503: "Service unavailable",
  504: "Request timeout",
});

export const WORKSPACE_INTEGRITY_ERROR = "workspace_integrity_error";
export const ROUTING_DEFAULTS_INVALID_ERROR = "managed_routing_defaults_invalid";

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
  AUTH_STATUS_ATTRIBUTE: "data-mpr-auth-status",
  CONFIG_URL_ATTRIBUTE: "data-config-url",
  HEADER_ID: "llm-proxy-header",
  HEADER_MISSING: "llm_proxy_mpr_ui_header_missing",
  HEADER_STATUS_MISSING: "llm_proxy_mpr_ui_header_status_missing",
  ORCHESTRATION_LOADER_MISSING: "llm_proxy_mpr_ui_orchestration_loader_missing",
  USER_SELECTOR: "mpr-user",
  USER_MENU_ITEMS_ATTRIBUTE: "menu-items",
  YAML_LOADER_MISSING: "llm_proxy_yaml_loader_missing",
});

export const COPY = Object.freeze({
  loadingEyebrow: "Session",
  loadingTitle: "Loading key workspace",
  signedOutEyebrow: "Authentication",
  signedOutTitle: "Sign in to manage LLM Proxy keys",
  profileErrorEyebrow: "Workspace",
  profileErrorTitle: "Unable to load key workspace",
  tenantSwitcherLabel: "Active tenant",
  tenantID: "Tenant ID",
  createTenant: "Create tenant",
  createTenantTitle: "Create tenant",
  tenantName: "Tenant name",
  tenantNameHint: "Use 1–80 visible characters. Names must be unique in your account.",
  tenantNameInvalid: "Enter a tenant name with 1–80 visible characters.",
  tenantNameConflict: "A tenant with that name already exists.",
  cancelCreateTenant: "Cancel",
  confirmCreateTenant: "Create",
  renameTenant: "Rename tenant",
  saveTenantName: "Save name",
  deleteTenant: "Delete tenant",
  deleteTenantTitle: "Delete tenant?",
  deleteTenantDescription: "This permanently deletes the tenant, its client key, provider settings, and usage history.",
  deleteTenantConfirmation: "Delete",
  cancelDeleteTenant: "Cancel",
  finalTenantDeletion: "Your final tenant cannot be deleted.",
  discardTenantChangesTitle: "Discard unsaved changes?",
  discardTenantChangesDescription: "Switching tenants discards unsaved Settings edits.",
  stayOnTenant: "Stay",
  discardAndSwitchTenant: "Discard and switch",
  invalidTenantURL: "The tenant in this URL is unavailable.",
  tenantCreated: "Tenant created",
  tenantRenamed: "Tenant renamed",
  tenantDeleted: "Tenant deleted",
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
  usageInterval: "Usage interval",
  usageEmpty: "No usage recorded",
  usageFailuresTitle: "Failed request details",
  usageFailuresDescription: "Failure metadata for the selected tenant and usage interval. Prompts, responses, provider errors, and credentials are never included.",
  usageFailuresStatusBreakdown: "Status breakdown",
  usageFailuresOccurredAt: "Occurred",
  usageFailuresEndpoint: "Endpoint",
  usageFailuresProvider: "Provider",
  usageFailuresModel: "Model",
  usageFailuresStatus: "Status",
  usageFailuresOutcome: "Outcome",
  usageFailuresLatency: "Latency",
  usageFailuresNotResolved: "Not resolved",
  usageFailuresLoading: "Loading failed requests",
  usageFailuresEmpty: "No failed requests in this interval",
  usageFailuresError: "Unable to load failed requests",
  usageFailuresRetry: "Retry",
  usageFailuresLoadMore: "Load more",
  closeUsageFailures: "Close failed request details",
  adminEmpty: "No managed users",
  adminTenant: "Tenant",
  adminSecret: "Secret",
  adminSecretReady: "Created",
  adminSecretMissing: "Missing",
  adminUpdated: "Updated",
  adminUserFallback: "Unnamed user",
  settingsEyebrow: "Settings",
  closeSettings: "Close",
  settingsRequiresClientAndProviderKey: "Create a client key and add at least one provider API key before leaving Settings.",
  settingsRequiresClientKey: "Create a client key before leaving Settings.",
  settingsRequiresProviderKey: "Add at least one provider API key before leaving Settings.",
  clientKey: "Key",
  clientKeyMissing: "No key created",
  clientKeyRetained: "This key is saved and can’t be shown again. Replace it to create and copy a new key.",
  createKey: "Create key",
  replaceKey: "Replace key",
  revokeKey: "Revoke key",
  showClientKey: "Show key",
  hideClientKey: "Hide key",
  copyClientKey: "Copy key",
  defaultsEyebrow: "Defaults",
  defaultsTitle: "Routing defaults",
  textProvider: "Text provider",
  textModel: "Text model",
  reasoningEffort: "Reasoning effort",
  reasoningEffortUnset: "Not set",
  reasoningEffortUnsupported: "Not supported",
  dictationProvider: "Dictation provider",
  dictationModel: "Dictation model",
  systemPrompt: "System prompt",
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
  providerSelector: "Provider",
  providerKeySuffix: " API key",
  showProviderKey: "Show key",
  hideProviderKey: "Hide key",
  providerTextModel: "Provider default model",
  providerSystemPrompt: "System prompt",
  removeProviderKey: "Remove provider key and settings",
  removeProviderKeyConfirmationTitle: "Remove provider key?",
  removeProviderKeyConfirmationMessage: "This removes the selected provider key and its settings. This action cannot be undone.",
  cancelProviderKeyRemoval: "Cancel",
  confirmProviderKeyRemoval: "Remove key",
  profileLoaded: "Workspace loaded",
  usageRefreshed: "Usage refreshed",
  providerSettingsSaved: "Provider settings saved",
  providerKeyRemoved: "Provider key and settings removed",
  defaultsSaved: "Defaults saved",
  keyGenerated: "Key created",
  keyRevoked: "Key revoked",
  keyCopied: "Key copied",
  exampleCopied: "Example copied",
  copyUnavailable: "Copy unavailable",
  authenticationRequired: "Authentication required",
  requestFailed: "Request failed",
  workspaceIntegrityError: "Workspace integrity error",
});
