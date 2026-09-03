// @ts-check

export const AUTH_STATES = Object.freeze({
  LOADING: "loading",
  AUTHENTICATED: "authenticated",
  UNAUTHENTICATED: "unauthenticated",
  ERROR: "error",
});

export const PUBLIC_SITE_PATH = "/";
export const APPLICATION_PATH = "/app/";
export const LANDING_AUTHENTICATED_REDIRECT_ATTRIBUTE = "data-llm-proxy-authenticated-redirect-url";

export const PUBLIC_THEME = Object.freeze({
  ATTRIBUTE: "data-mpr-theme",
  PALETTE_ATTRIBUTE: "data-llm-proxy-palette",
});

export const NOTICE_KINDS = Object.freeze({
  INFO: "info",
  SUCCESS: "success",
  ERROR: "error",
});

export const NOTICE_SURFACES = Object.freeze({
  HEADER: "header",
  SETTINGS: "settings",
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

export const CAPABILITY_CATALOG_SORTS = Object.freeze({
  PUBLISHER: "publisher",
  MODEL: "model",
  CAPABILITIES: "capabilities",
});

export const CAPABILITY_CATALOG_SORT_DIRECTIONS = Object.freeze({
  ASCENDING: "ascending",
  DESCENDING: "descending",
});

export const CAPABILITY_CATALOG_COPY = Object.freeze({
  RESULT_SEPARATOR: "of",
  MODEL: "model",
  MODELS: "models",
  SORT_BY: "Sort by",
});

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
  499: "Client closed request",
  502: "Upstream error",
  503: "Service unavailable",
  504: "Request timeout",
});

export const APP_INTEGRITY_ERROR = "app_integrity_error";
export const ROUTING_DEFAULTS_INVALID_ERROR = "managed_routing_defaults_invalid";
export const PROVIDER_KEY_VERIFICATION_ERRORS = Object.freeze({
  REJECTED: "provider_key_rejected",
  RATE_LIMITED: "provider_key_verification_rate_limited",
  TIMED_OUT: "provider_key_verification_timed_out",
  UNAVAILABLE: "provider_key_verification_unavailable",
});

export const PROVIDER_CAPABILITY_LABELS = Object.freeze({
  text: "Text",
  image_input: "Image analysis",
  audio_input: "Audio analysis",
  dictation: "Dictation",
  video_generation: "Video generation",
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

export const RUNTIME_UI = Object.freeze({
  ALPINE_RUNTIME_MODULE_URL: "/assets/llm-proxy/js/alpineRuntime.js?v=20260902c240",
  APPLICATION_MODULE_ID: "llm-proxy-application-module",
  APPLICATION_MODULE_MISSING: "llm_proxy_application_module_missing",
  APPLICATION_READY_ATTRIBUTE: "data-llm-proxy-application",
  FAILURE_DESCRIPTION_ID: "llm-proxy-runtime-failure-description",
  FAILURE_EYEBROW_ID: "llm-proxy-runtime-failure-eyebrow",
  FAILURE_RELOAD_ID: "llm-proxy-runtime-failure-reload",
  FAILURE_SURFACE_ID: "llm-proxy-runtime-failure",
  FAILURE_SURFACE_MISSING: "llm_proxy_runtime_failure_surface_missing",
  FAILURE_TITLE_ID: "llm-proxy-runtime-failure-title",
  GUARD_READY_ATTRIBUTE: "data-llm-proxy-startup-guard",
  STARTUP_ERROR_ATTRIBUTE: "data-llm-proxy-startup-error",
  TRANSITION_COMPLETION_FAILED_ATTRIBUTE: "data-mpr-transition-completion-failed",
});

export const COPY = Object.freeze({
  runtimeFailureEyebrow: "Application startup",
  runtimeFailureTitle: "Unable to open LLM Proxy",
  runtimeFailureDescription: "Your browser could not load the current application files. Allow this site and cdn.jsdelivr.net in browser controls, then reload.",
  runtimeFailureReload: "Reload LLM Proxy",
  loadingEyebrow: "Session",
  loadingTitle: "Loading LLM Proxy",
  profileErrorEyebrow: "App",
  profileErrorTitle: "Unable to load LLM Proxy",
  usageTenant: "Usage tenant",
  allTenants: "All tenants",
  tenantAccess: "Tenant access",
  createTenant: "Create tenant",
  createTenantTitle: "Create tenant",
  tenantName: "Tenant name",
  tenantNameHint: "Use 1–80 visible characters. Names must be unique in your account.",
  tenantNameInvalid: "Enter a tenant name with 1–80 visible characters.",
  tenantNameConflict: "A tenant with that name already exists.",
  cancelCreateTenant: "Cancel",
  confirmCreateTenant: "Create",
  tenantContext: "Tenant",
  renameTenant: "Rename tenant",
  renameTenantAction: "Rename",
  cancelTenantName: "Cancel",
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
  usageProviders: "Providers used",
  usageRequestTrend: "Requests",
  usageTokenTrend: "Tokens",
  usageByProvider: "Provider usage",
  usageByModel: "Model usage",
  usageBreakdownView: "Breakdown view",
  usageBreakdownBar: "Bar graph",
  usageBreakdownDonut: "Donut chart",
  usageBreakdownRequests: "requests",
  usageProviderRequestShares: "Provider request shares",
  usageModelRequestShares: "Model request shares",
  usageChartTokensValue: "tokens",
  usageTimeAxis: "Time (UTC)",
  usageRequestsPerHour: "Requests per hour",
  usageRequestsPerDay: "Requests per day",
  usageTokensPerHour: "Tokens per hour",
  usageTokensPerDay: "Tokens per day",
  usageInterval: "Usage interval",
  usageEmpty: "No usage recorded",
  usageFailuresTitle: "Failed request details",
  usageFailuresDescription: "Failure metadata for the selected usage scope and interval. Prompts, responses, provider errors, and credentials are never included.",
  usageFailuresTenant: "Tenant",
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
  clientKeyRetained: "Saved; replace to reveal a new key.",
  createKey: "Create key",
  replaceKey: "Replace key",
  replaceKeyTitle: "Replace client key?",
  replaceKeyDescription: "The current key will stop working immediately. Copy the replacement now; it cannot be shown again.",
  cancelReplaceKey: "Cancel",
  confirmReplaceKey: "Replace key",
  showClientKey: "Show key",
  hideClientKey: "Hide key",
  copyClientKey: "Copy key",
  defaultsEyebrow: "Defaults",
  defaultsTitle: "Routing defaults",
  routingDefaultsHelpLabel: "About routing defaults",
  routingDefaultsHelp: "Used when a request omits both provider and model. Selecting a text provider starts with its provider default model; you can then select another routing model.",
  textProvider: "Text provider",
  textModel: "Text model",
  reasoningEffort: "Reasoning effort",
  reasoningEffortUnset: "Not set",
  reasoningEffortUnsupported: "Not supported",
  dictationProvider: "Dictation provider",
  dictationModel: "Dictation model",
  routingDefaultUnavailable: "Not configured",
  systemPrompt: "System prompt",
  systemPromptHidden: "Hidden",
  systemPromptExpanded: "Expanded",
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
  providerCardsTitle: "API connections",
  providerCardsDescription: "Usage, model identities, and tenant-specific API key settings.",
  providerModelFamilies: "Model families",
  providerCapabilities: "Capabilities",
  providerRequests: "Requests",
  providerRequestVolume: "Request volume",
  providerRequestGraphScale: "of the highest provider request count",
  providerTokens: "Tokens",
  providerActive: "active",
  providerUsed: "used",
  providerUnavailable: "Unavailable",
  providerTenant: "Tenant",
  providerSetKey: "Set API key",
  providerKeySettings: "API key settings",
  providerGetKey: "Get API key",
  providerClose: "Close provider settings",
  providerSettingsLoading: "Loading provider settings...",
  providerSelector: "Provider",
  showProviderKey: "Show key",
  hideProviderKey: "Hide key",
  providerTextModel: "Provider default model",
  providerDefaultModelHelpLabel: "About provider default model",
  providerDefaultModelHelp: "Used when a request selects this provider but omits a model. If this provider is also the routing default, changing this model updates that route; you can then override the routing model.",
  providerSystemPrompt: "System prompt",
  removeProviderKey: "Delete key",
  removeProviderKeyConfirmationTitle: "Remove provider key?",
  removeProviderKeyConfirmationMessage: "This deletes only the saved provider key. The provider model, prompt, and usage history stay available.",
  cancelProviderKeyRemoval: "Cancel",
  confirmProviderKeyRemoval: "Remove key",
  profileLoaded: "App ready",
  usageRefreshed: "Usage refreshed",
  providerSettingsSaved: "Provider settings saved",
  providerKeyVerifying: "Checking key...",
  providerKeyVerified: "Provider key verified and settings saved",
  providerKeyRetry: "Retry verification",
  providerKeyRejectedUnsaved: "Key was rejected. No provider key was saved.",
  providerKeyRejectedPreviousActive: "Key was rejected. The previous key remains active.",
  providerKeyRateLimitedUnsaved: "Key could not be verified because the provider rate limit was reached. No provider key was saved.",
  providerKeyRateLimitedPreviousActive: "Key could not be verified because the provider rate limit was reached. The previous key remains active.",
  providerKeyTimedOutUnsaved: "Key verification timed out. No provider key was saved.",
  providerKeyTimedOutPreviousActive: "Key verification timed out. The previous key remains active.",
  providerKeyUnavailableUnsaved: "The provider could not verify this key. No provider key was saved.",
  providerKeyUnavailablePreviousActive: "The provider could not verify this key. The previous key remains active.",
  providerKeyRemoved: "Provider key deleted",
  defaultsSaved: "Defaults saved",
  keyGenerated: "Key created",
  keyReplaced: "Key replaced",
  keyCopied: "Key copied",
  exampleCopied: "Example copied",
  copyUnavailable: "Copy unavailable",
  authenticationRequired: "Authentication required",
  requestFailed: "Request failed",
  appIntegrityError: "App data integrity error",
});
