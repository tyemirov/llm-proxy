package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/constants"
	"go.uber.org/zap"
)

const (
	managementAPIPath                   = "/api/management"
	managementAccountPath               = "/account"
	managementTenantsPath               = "/tenants"
	managementTenantPath                = managementTenantsPath + "/:tenant_id"
	managementProviderKeysPath          = "/provider-keys/:provider"
	managementProviderKeyRevealPath     = managementProviderKeysPath + "/reveal"
	managementDefaultsPath              = "/defaults"
	managementSecretsPath               = "/secrets"
	managementUsagePath                 = "/usage"
	managementUsageFailuresPath         = managementUsagePath + "/failures"
	managementAdminUsersPath            = "/admin/users"
	contextKeyManagementPrincipal       = "management_principal"
	headerAccessControlAllowCredentials = "Access-Control-Allow-Credentials"
	headerAccessControlAllowHeaders     = "Access-Control-Allow-Headers"
	headerAccessControlAllowMethods     = "Access-Control-Allow-Methods"
	headerAccessControlAllowOrigin      = "Access-Control-Allow-Origin"
	headerCacheControl                  = "Cache-Control"
	headerOrigin                        = "Origin"
	headerVary                          = "Vary"
	cacheControlNoStore                 = "no-store"
	mimeApplicationYAML                 = "application/yaml; charset=utf-8"
)

var (
	errManagementBadRequest = errors.New("management_bad_request")
	errManagementDefaults   = errors.New("management_defaults_invalid")
)

type managementService struct {
	configuration    ManagementConfiguration
	sessionValidator *managementSessionValidator
	store            *managedTenantStore
	providers        *providerRegistry
	keyVerifier      providerKeyVerifier
	authenticator    tenantAuthenticator
	structuredLogger *zap.SugaredLogger
}

type managementAccountResponse struct {
	User    managementUserResponse            `json:"user"`
	Tenants []managementTenantSummaryResponse `json:"tenants"`
}

type managementTenantProfileResponse struct {
	Tenant    managementTenantResponse     `json:"tenant"`
	Providers []managementProviderResponse `json:"providers"`
	Proxy     managementProxyResponse      `json:"proxy"`
}

type managementUserResponse struct {
	ID          string `json:"id"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	IsAdmin     bool   `json:"is_admin"`
}

type managementTenantResponse struct {
	ID        string                           `json:"id"`
	Name      string                           `json:"name"`
	HasSecret bool                             `json:"has_secret"`
	Defaults  managementTenantDefaultsResponse `json:"defaults"`
	CreatedAt string                           `json:"created_at"`
	UpdatedAt string                           `json:"updated_at"`
}

type managementTenantSummaryResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	HasSecret bool   `json:"has_secret"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type managementTenantDefaultsResponse struct {
	Provider          string `json:"provider"`
	Model             string `json:"model"`
	DictationProvider string `json:"dictation_provider"`
	DictationModel    string `json:"dictation_model"`
	SystemPrompt      string `json:"system_prompt"`
	ReasoningEffort   string `json:"reasoning_effort"`
}

type managementProviderResponse struct {
	ID                    string                        `json:"id"`
	Label                 string                        `json:"label"`
	Aliases               []string                      `json:"aliases"`
	HasKey                bool                          `json:"has_key"`
	MaskedKey             string                        `json:"masked_key,omitempty"`
	BaseURL               string                        `json:"base_url"`
	TextModel             string                        `json:"text_model"`
	SystemPrompt          string                        `json:"system_prompt"`
	TextDefaultModel      string                        `json:"text_default_model"`
	TextModels            []managementTextModelResponse `json:"text_models"`
	SupportsDictation     bool                          `json:"supports_dictation"`
	DictationDefaultModel string                        `json:"dictation_default_model,omitempty"`
	DictationModels       []string                      `json:"dictation_models"`
}

type managementTextModelResponse struct {
	ID              string                                       `json:"id"`
	ReasoningEffort *managementReasoningEffortCapabilityResponse `json:"reasoning_effort,omitempty"`
}

type managementReasoningEffortCapabilityResponse struct {
	Adapter string   `json:"adapter"`
	Efforts []string `json:"efforts"`
}

type managementProxyResponse struct {
	TextPath      string `json:"text_path"`
	V2Path        string `json:"v2_path"`
	DictationPath string `json:"dictation_path"`
}

type managementSecretResponse struct {
	Secret  string                          `json:"secret"`
	Profile managementTenantProfileResponse `json:"profile"`
}

type managementProviderKeyRevealResponse struct {
	APIKey string `json:"api_key"`
}

type managementUsageSummaryResponse struct {
	Interval    string                            `json:"interval"`
	BucketUnit  string                            `json:"bucket_unit"`
	Totals      managementUsageAggregateResponse  `json:"totals"`
	Buckets     []managementUsageBucketResponse   `json:"buckets"`
	Providers   []managementUsageProviderResponse `json:"providers"`
	Models      []managementUsageModelResponse    `json:"models"`
	StatusCodes []managementUsageStatusResponse   `json:"status_codes"`
}

type managementUsageFailuresResponse struct {
	Interval   string                           `json:"interval"`
	Failures   []managementUsageFailureResponse `json:"failures"`
	NextCursor string                           `json:"next_cursor,omitempty"`
}

type managementAccountUsageFailuresResponse struct {
	Interval   string                                  `json:"interval"`
	Failures   []managementAccountUsageFailureResponse `json:"failures"`
	NextCursor string                                  `json:"next_cursor,omitempty"`
}

type managementUsageFailureResponse struct {
	OccurredAt          string `json:"occurred_at"`
	Endpoint            string `json:"endpoint"`
	Provider            string `json:"provider"`
	Model               string `json:"model"`
	StatusCode          int    `json:"status_code"`
	OutcomeCode         string `json:"outcome_code"`
	LatencyMilliseconds int64  `json:"latency_ms"`
}

type managementAccountUsageFailureResponse struct {
	TenantID   string `json:"tenant_id"`
	TenantName string `json:"tenant_name"`
	managementUsageFailureResponse
}

type managementAdminUsageSummaryResponse struct {
	PeriodDays  int                               `json:"period_days"`
	Totals      managementUsageAggregateResponse  `json:"totals"`
	Daily       []managementUsageDailyResponse    `json:"daily"`
	Providers   []managementUsageProviderResponse `json:"providers"`
	Models      []managementUsageModelResponse    `json:"models"`
	StatusCodes []managementUsageStatusResponse   `json:"status_codes"`
}

type managementUsageAggregateResponse struct {
	Requests                   int   `json:"requests"`
	SuccessfulRequests         int   `json:"successful_requests"`
	FailedRequests             int   `json:"failed_requests"`
	TextRequests               int   `json:"text_requests"`
	DictationRequests          int   `json:"dictation_requests"`
	RequestTokens              int   `json:"request_tokens"`
	ResponseTokens             int   `json:"response_tokens"`
	TotalTokens                int   `json:"total_tokens"`
	AverageLatencyMilliseconds int64 `json:"average_latency_ms"`
}

type managementUsageDailyResponse struct {
	Date string                           `json:"date"`
	Data managementUsageAggregateResponse `json:"data"`
}

type managementUsageBucketResponse struct {
	Start string                           `json:"start"`
	Data  managementUsageAggregateResponse `json:"data"`
}

type managementUsageProviderResponse struct {
	Provider string                           `json:"provider"`
	Data     managementUsageAggregateResponse `json:"data"`
}

type managementUsageModelResponse struct {
	Provider string                           `json:"provider"`
	Model    string                           `json:"model"`
	Data     managementUsageAggregateResponse `json:"data"`
}

type managementUsageStatusResponse struct {
	StatusCode int `json:"status_code"`
	Requests   int `json:"requests"`
}

type managementAdminUsersResponse struct {
	PeriodDays int                           `json:"period_days"`
	Users      []managementAdminUserResponse `json:"users"`
}

type managementAdminUserResponse struct {
	User        managementUserResponse          `json:"user"`
	TenantCount int                             `json:"tenant_count"`
	Tenants     []managementAdminTenantResponse `json:"tenants"`
}

type managementAdminTenantResponse struct {
	ID        string                              `json:"id"`
	Name      string                              `json:"name"`
	HasSecret bool                                `json:"has_secret"`
	CreatedAt string                              `json:"created_at"`
	UpdatedAt string                              `json:"updated_at"`
	Usage     managementAdminUsageSummaryResponse `json:"usage"`
}

type managementTenantNameRequest struct {
	Name string `json:"name"`
}

type managementProviderKeyRequest struct {
	APIKey       string `json:"api_key"`
	BaseURL      string `json:"base_url"`
	TextModel    string `json:"text_model"`
	SystemPrompt string `json:"system_prompt"`
}

type managementDefaultsRequest struct {
	Provider          string  `json:"provider"`
	Model             string  `json:"model"`
	DictationProvider string  `json:"dictation_provider"`
	DictationModel    string  `json:"dictation_model"`
	SystemPrompt      string  `json:"system_prompt"`
	ReasoningEffort   *string `json:"reasoning_effort"`
}

func newManagementService(configuration ManagementConfiguration, sessionValidator *managementSessionValidator, store *managedTenantStore, providers *providerRegistry, keyVerifier providerKeyVerifier, authenticator tenantAuthenticator, structuredLogger *zap.SugaredLogger) *managementService {
	return &managementService{
		configuration:    configuration,
		sessionValidator: sessionValidator,
		store:            store,
		providers:        providers,
		keyVerifier:      keyVerifier,
		authenticator:    authenticator,
		structuredLogger: structuredLogger,
	}
}

func (service *managementService) registerRoutes(router *gin.Engine) {
	router.GET(ManagementConfigUIPath, service.corsMiddleware(), service.configUIHandler())
	router.OPTIONS(ManagementConfigUIPath, service.corsMiddleware(), service.corsPreflightHandler())

	managementGroup := router.Group(managementAPIPath)
	managementGroup.Use(service.corsMiddleware())
	managementGroup.OPTIONS("/*path", service.corsPreflightHandler())
	managementGroup.Use(service.sessionMiddleware())
	managementGroup.Use(service.managementMutationMiddleware())
	managementGroup.GET(managementAccountPath, service.accountHandler())
	managementGroup.GET(managementUsagePath, service.accountUsageHandler())
	managementGroup.GET(managementUsageFailuresPath, service.accountUsageFailuresHandler())
	managementGroup.POST(managementTenantsPath, service.createTenantHandler())
	managementGroup.GET(managementAdminUsersPath, service.adminUsersHandler())

	tenantGroup := managementGroup.Group(managementTenantPath)
	tenantGroup.GET("", service.tenantProfileHandler())
	tenantGroup.PUT("", service.renameTenantHandler())
	tenantGroup.DELETE("", service.deleteTenantHandler())
	tenantGroup.GET(managementUsagePath, service.usageHandler())
	tenantGroup.GET(managementUsageFailuresPath, service.usageFailuresHandler())
	tenantGroup.PUT(managementProviderKeysPath, service.saveProviderKeyHandler())
	tenantGroup.DELETE(managementProviderKeysPath, service.removeProviderKeyHandler())
	tenantGroup.POST(managementProviderKeyRevealPath, service.managementCredentialedActionMiddleware(), service.revealProviderKeyHandler())
	tenantGroup.PUT(managementDefaultsPath, service.updateDefaultsHandler())
	tenantGroup.POST(managementSecretsPath, service.generateSecretHandler())
}

func (service *managementService) sessionMiddleware() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		principal, validationError := service.sessionValidator.validateRequest(ginContext.Request)
		if validationError != nil {
			service.structuredLogger.Warnw("management session rejected", "reason", managementSessionRejectionReason(validationError))
			ginContext.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		ginContext.Set(contextKeyManagementPrincipal, principal)
		ginContext.Next()
	}
}

func (service *managementService) configUIHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		ginContext.Header(headerCacheControl, cacheControlNoStore)
		ginContext.Data(http.StatusOK, mimeApplicationYAML, []byte(RenderManagementConfigUI(service.configuration)))
	}
}

func (service *managementService) corsMiddleware() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		service.applyCORSHeaders(ginContext)
		ginContext.Next()
	}
}

func (service *managementService) managementMutationMiddleware() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		if !managementMethodUnsafe(ginContext.Request.Method) {
			ginContext.Next()
			return
		}
		requestOrigin := strings.TrimSpace(ginContext.GetHeader(headerOrigin))
		if requestOrigin != constants.EmptyString && requestOrigin != service.configuration.PublicOrigin {
			ginContext.AbortWithStatus(http.StatusForbidden)
			return
		}
		if !managementRequestJSON(ginContext.GetHeader(headerContentType)) {
			ginContext.AbortWithStatus(http.StatusUnsupportedMediaType)
			return
		}
		ginContext.Next()
	}
}

func (service *managementService) managementCredentialedActionMiddleware() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		if strings.TrimSpace(ginContext.GetHeader(headerOrigin)) != service.configuration.PublicOrigin {
			ginContext.AbortWithStatus(http.StatusForbidden)
			return
		}
		ginContext.Next()
	}
}

func (service *managementService) corsPreflightHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		if strings.TrimSpace(ginContext.GetHeader(headerOrigin)) != service.configuration.PublicOrigin {
			ginContext.AbortWithStatus(http.StatusForbidden)
			return
		}
		ginContext.AbortWithStatus(http.StatusNoContent)
	}
}

func (service *managementService) applyCORSHeaders(ginContext *gin.Context) {
	requestOrigin := strings.TrimSpace(ginContext.GetHeader(headerOrigin))
	if requestOrigin == "" || requestOrigin != service.configuration.PublicOrigin {
		return
	}
	ginContext.Header(headerAccessControlAllowOrigin, requestOrigin)
	ginContext.Header(headerAccessControlAllowCredentials, "true")
	ginContext.Header(headerAccessControlAllowHeaders, headerContentType)
	ginContext.Header(headerAccessControlAllowMethods, "GET, PUT, POST, DELETE, OPTIONS")
	ginContext.Header(headerVary, headerOrigin)
}

func (service *managementService) accountHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		principal := managementPrincipalFromContext(ginContext)
		snapshot, snapshotError := service.store.account(principal)
		if snapshotError != nil {
			ginContext.String(http.StatusInternalServerError, snapshotError.Error())
			return
		}
		tenants := make([]managementTenantSummaryResponse, 0, len(snapshot.tenants))
		for _, tenantSummary := range snapshot.tenants {
			tenants = append(tenants, managementTenantSummaryResponse{
				ID:        tenantSummary.tenantID,
				Name:      tenantSummary.name,
				HasSecret: tenantSummary.hasSecret,
				CreatedAt: tenantSummary.createdAt.Format(time.RFC3339),
				UpdatedAt: tenantSummary.updatedAt.Format(time.RFC3339),
			})
		}
		ginContext.Header(headerCacheControl, cacheControlNoStore)
		ginContext.JSON(http.StatusOK, managementAccountResponse{
			User: managementUserResponse{
				ID:          snapshot.userID,
				Email:       snapshot.userEmail,
				DisplayName: snapshot.userDisplayName,
				AvatarURL:   snapshot.userAvatarURL,
				IsAdmin:     principal.isAdmin,
			},
			Tenants: tenants,
		})
	}
}

func (service *managementService) createTenantHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		var request managementTenantNameRequest
		if decodeError := decodeManagementJSON(ginContext, &request); decodeError != nil {
			ginContext.String(http.StatusBadRequest, decodeError.Error())
			return
		}
		name, nameError := newManagedTenantName(request.Name)
		if nameError != nil {
			ginContext.String(http.StatusBadRequest, nameError.Error())
			return
		}
		snapshot, createError := service.store.createTenant(managementPrincipalFromContext(ginContext), name)
		if createError != nil {
			writeManagementStoreError(ginContext, createError)
			return
		}
		service.writeTenantProfileResponse(ginContext, snapshot, http.StatusCreated)
	}
}

func (service *managementService) tenantProfileHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		tenantIdentifier, identifierValid := managementTenantIdentifierFromContext(ginContext)
		if !identifierValid {
			return
		}
		snapshot, snapshotError := service.store.tenantProfile(managementPrincipalFromContext(ginContext), tenantIdentifier)
		if snapshotError != nil {
			writeManagementStoreError(ginContext, snapshotError)
			return
		}
		service.writeTenantProfileResponse(ginContext, snapshot, http.StatusOK)
	}
}

func (service *managementService) renameTenantHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		tenantIdentifier, identifierValid := managementTenantIdentifierFromContext(ginContext)
		if !identifierValid {
			return
		}
		var request managementTenantNameRequest
		if decodeError := decodeManagementJSON(ginContext, &request); decodeError != nil {
			ginContext.String(http.StatusBadRequest, decodeError.Error())
			return
		}
		name, nameError := newManagedTenantName(request.Name)
		if nameError != nil {
			ginContext.String(http.StatusBadRequest, nameError.Error())
			return
		}
		snapshot, renameError := service.store.renameTenant(managementPrincipalFromContext(ginContext), tenantIdentifier, name)
		if renameError != nil {
			writeManagementStoreError(ginContext, renameError)
			return
		}
		service.writeTenantProfileResponse(ginContext, snapshot, http.StatusOK)
	}
}

func (service *managementService) deleteTenantHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		tenantIdentifier, identifierValid := managementTenantIdentifierFromContext(ginContext)
		if !identifierValid {
			return
		}
		if deleteError := service.store.deleteTenant(managementPrincipalFromContext(ginContext), tenantIdentifier); deleteError != nil {
			writeManagementStoreError(ginContext, deleteError)
			return
		}
		ginContext.Status(http.StatusNoContent)
	}
}

func (service *managementService) usageHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		tenantIdentifier, identifierValid := managementTenantIdentifierFromContext(ginContext)
		if !identifierValid {
			return
		}
		interval, intervalValid := managementUsageInterval(ginContext)
		if !intervalValid {
			return
		}
		principal := managementPrincipalFromContext(ginContext)
		summary, summaryError := service.store.usageSummary(principal, tenantIdentifier, interval)
		if summaryError != nil {
			writeManagementStoreError(ginContext, summaryError)
			return
		}
		ginContext.Header(headerCacheControl, cacheControlNoStore)
		ginContext.JSON(http.StatusOK, managementUsageSummary(summary))
	}
}

func (service *managementService) accountUsageHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		interval, intervalValid := managementUsageInterval(ginContext)
		if !intervalValid {
			return
		}
		summary, summaryError := service.store.accountUsageSummary(managementPrincipalFromContext(ginContext), interval)
		if summaryError != nil {
			writeManagementStoreError(ginContext, summaryError)
			return
		}
		ginContext.Header(headerCacheControl, cacheControlNoStore)
		ginContext.JSON(http.StatusOK, managementUsageSummary(summary))
	}
}

func (service *managementService) usageFailuresHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		tenantIdentifier, identifierValid := managementTenantIdentifierFromContext(ginContext)
		if !identifierValid {
			return
		}
		query, queryError := newManagedUsageFailureQuery(ginContext.Request.URL.Query(), tenantIdentifier.string())
		if queryError != nil {
			ginContext.String(http.StatusBadRequest, queryError.Error())
			return
		}
		page, pageError := service.store.usageFailures(managementPrincipalFromContext(ginContext), tenantIdentifier, query)
		if pageError != nil {
			writeManagementStoreError(ginContext, pageError)
			return
		}
		ginContext.Header(headerCacheControl, cacheControlNoStore)
		ginContext.JSON(http.StatusOK, managementUsageFailuresResponseFor(page))
	}
}

func (service *managementService) accountUsageFailuresHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		query, queryError := newManagedUsageFailureQuery(ginContext.Request.URL.Query(), managedUsageAllTenantsScope)
		if queryError != nil {
			ginContext.String(http.StatusBadRequest, queryError.Error())
			return
		}
		page, pageError := service.store.accountUsageFailures(managementPrincipalFromContext(ginContext), query)
		if pageError != nil {
			writeManagementStoreError(ginContext, pageError)
			return
		}
		ginContext.Header(headerCacheControl, cacheControlNoStore)
		ginContext.JSON(http.StatusOK, managementAccountUsageFailuresResponseFor(page))
	}
}

func managementUsageInterval(ginContext *gin.Context) (usageInterval, bool) {
	query := ginContext.Request.URL.Query()
	intervalValues, intervalExists := query["interval"]
	if len(query) != 1 || !intervalExists || len(intervalValues) != 1 {
		ginContext.String(http.StatusBadRequest, errManagedUsageIntervalInvalid.Error())
		return "", false
	}
	interval, intervalError := newUsageInterval(intervalValues[0])
	if intervalError != nil {
		ginContext.String(http.StatusBadRequest, intervalError.Error())
		return "", false
	}
	return interval, true
}

func (service *managementService) adminUsersHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		principal := managementPrincipalFromContext(ginContext)
		if !principal.isAdmin {
			ginContext.AbortWithStatus(http.StatusForbidden)
			return
		}
		summary, summaryError := service.store.adminUsersSummary()
		if summaryError != nil {
			ginContext.String(http.StatusInternalServerError, summaryError.Error())
			return
		}
		ginContext.JSON(http.StatusOK, service.adminUsersResponse(summary))
	}
}

func (service *managementService) saveProviderKeyHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		tenantIdentifier, identifierValid := managementTenantIdentifierFromContext(ginContext)
		if !identifierValid {
			return
		}
		principal := managementPrincipalFromContext(ginContext)
		providerIdentifier, providerError := service.providers.canonicalProviderID(ginContext.Param("provider"))
		if providerError != nil {
			ginContext.String(http.StatusBadRequest, providerError.Error())
			return
		}
		var request managementProviderKeyRequest
		if decodeError := decodeManagementJSON(ginContext, &request); decodeError != nil {
			ginContext.String(http.StatusBadRequest, decodeError.Error())
			return
		}
		provider, textModel, providerSettingsError := service.resolveManagedProviderSettings(providerIdentifier, request)
		if providerSettingsError != nil {
			ginContext.String(http.StatusBadRequest, providerSettingsError.Error())
			return
		}
		currentSnapshot, storeError := service.store.tenantProfile(principal, tenantIdentifier)
		if storeError != nil {
			writeManagementStoreError(ginContext, storeError)
			return
		}
		verificationAPIKey := strings.TrimSpace(request.APIKey)
		var verifiedAPIKeyVersion *managedProviderKeyVersion
		if currentSettings, configured := currentSnapshot.providerSettings[providerIdentifier]; verificationAPIKey == constants.EmptyString && configured && providerIdentifier.string() == ProviderNameDashScope && currentSettings.baseURL != provider.textBaseURL {
			verificationAPIKey = currentSettings.apiKey
			verifiedAPIKeyVersion = &currentSettings.apiKeyVersion
		}
		if verificationAPIKey != constants.EmptyString {
			if verificationError := service.keyVerifier.verify(ginContext.Request.Context(), provider, textModel, verificationAPIKey); verificationError != nil {
				writeProviderKeyVerificationError(ginContext, verificationError)
				return
			}
		}
		snapshot, storeError := service.store.saveProviderKeyWithVerifiedVersion(ginContext.Request.Context(), principal, tenantIdentifier, providerIdentifier, request.APIKey, request.BaseURL, request.TextModel, request.SystemPrompt, verifiedAPIKeyVersion)
		if storeError != nil {
			writeManagementStoreError(ginContext, storeError)
			return
		}
		service.writeTenantProfileResponse(ginContext, snapshot, http.StatusOK)
	}
}

func (service *managementService) removeProviderKeyHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		tenantIdentifier, identifierValid := managementTenantIdentifierFromContext(ginContext)
		if !identifierValid {
			return
		}
		principal := managementPrincipalFromContext(ginContext)
		providerIdentifier, providerError := service.providers.canonicalProviderID(ginContext.Param("provider"))
		if providerError != nil {
			ginContext.String(http.StatusBadRequest, providerError.Error())
			return
		}
		snapshot, storeError := service.store.removeProviderKey(principal, tenantIdentifier, providerIdentifier)
		if storeError != nil {
			writeManagementStoreError(ginContext, storeError)
			return
		}
		service.writeTenantProfileResponse(ginContext, snapshot, http.StatusOK)
	}
}

func (service *managementService) revealProviderKeyHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		ginContext.Header(headerCacheControl, cacheControlNoStore)
		tenantIdentifier, identifierValid := managementTenantIdentifierFromContext(ginContext)
		if !identifierValid {
			return
		}
		providerIdentifier, providerError := service.providers.canonicalProviderID(ginContext.Param("provider"))
		if providerError != nil {
			ginContext.String(http.StatusBadRequest, providerError.Error())
			return
		}
		apiKey, revealError := service.store.revealProviderKey(managementPrincipalFromContext(ginContext), tenantIdentifier, providerIdentifier)
		if revealError != nil {
			if errors.Is(revealError, errManagedProviderKeyNotFound) || errors.Is(revealError, errManagedTenantNotFound) {
				ginContext.AbortWithStatus(http.StatusNotFound)
				return
			}
			ginContext.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		ginContext.JSON(http.StatusOK, managementProviderKeyRevealResponse{APIKey: apiKey})
	}
}

func (service *managementService) updateDefaultsHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		tenantIdentifier, identifierValid := managementTenantIdentifierFromContext(ginContext)
		if !identifierValid {
			return
		}
		principal := managementPrincipalFromContext(ginContext)
		var request managementDefaultsRequest
		if decodeError := decodeManagementJSON(ginContext, &request); decodeError != nil {
			ginContext.String(http.StatusBadRequest, decodeError.Error())
			return
		}
		rawDefaults, defaultsRequestError := request.tenantDefaults()
		if defaultsRequestError != nil {
			ginContext.String(http.StatusBadRequest, defaultsRequestError.Error())
			return
		}
		defaults, defaultsConstructionError := newManagedRoutingDefaults(service.providers, rawDefaults)
		if defaultsConstructionError != nil {
			ginContext.String(http.StatusBadRequest, defaultsConstructionError.Error())
			return
		}
		currentSnapshot, snapshotError := service.store.tenantProfile(principal, tenantIdentifier)
		if snapshotError != nil {
			writeManagementStoreError(ginContext, snapshotError)
			return
		}
		if defaultsError := service.validateManagedRoutingDefaults(currentSnapshot.providerSettings, defaults); defaultsError != nil {
			ginContext.String(http.StatusBadRequest, defaultsError.Error())
			return
		}
		snapshot, storeError := service.store.updateDefaults(principal, tenantIdentifier, defaults)
		if storeError != nil {
			writeManagementStoreError(ginContext, storeError)
			return
		}
		service.writeTenantProfileResponse(ginContext, snapshot, http.StatusOK)
	}
}

func (service *managementService) generateSecretHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		tenantIdentifier, identifierValid := managementTenantIdentifierFromContext(ginContext)
		if !identifierValid {
			return
		}
		principal := managementPrincipalFromContext(ginContext)
		rawSecret, snapshot, generationError := service.store.generateSecret(principal, tenantIdentifier, service.authenticator.containsStaticSecretDigest)
		if generationError != nil {
			writeManagementStoreError(ginContext, generationError)
			return
		}
		profile, profileError := service.tenantProfileResponse(snapshot)
		if profileError != nil {
			ginContext.String(http.StatusInternalServerError, profileError.Error())
			return
		}
		ginContext.Header(headerCacheControl, cacheControlNoStore)
		ginContext.JSON(http.StatusOK, managementSecretResponse{
			Secret:  rawSecret,
			Profile: profile,
		})
	}
}

func (service *managementService) writeTenantProfileResponse(ginContext *gin.Context, snapshot managedTenantSnapshot, statusCode int) {
	profile, profileError := service.tenantProfileResponse(snapshot)
	if profileError != nil {
		ginContext.String(http.StatusInternalServerError, profileError.Error())
		return
	}
	ginContext.Header(headerCacheControl, cacheControlNoStore)
	ginContext.JSON(statusCode, profile)
}

func (service *managementService) tenantProfileResponse(snapshot managedTenantSnapshot) (managementTenantProfileResponse, error) {
	defaults, defaultsError := validatePersistedManagedRoutingDefaults(service.providers, snapshot.providerSettings, snapshot.defaults)
	if defaultsError != nil {
		return managementTenantProfileResponse{}, fmt.Errorf("%w: tenant=%s: %w", errManagedRoutingDefaultsInvalid, snapshot.tenantID, defaultsError)
	}
	return managementTenantProfileResponse{
		Tenant: managementTenantResponse{
			ID:        snapshot.tenantID,
			Name:      snapshot.tenantName,
			HasSecret: snapshot.hasSecret,
			Defaults:  managementDefaultsResponse(defaults),
			CreatedAt: snapshot.createdAt.Format(time.RFC3339),
			UpdatedAt: snapshot.updatedAt.Format(time.RFC3339),
		},
		Providers: service.providerResponses(snapshot.providerSettings),
		Proxy: managementProxyResponse{
			TextPath:      rootPath,
			V2Path:        v2Path,
			DictationPath: dictatePath,
		},
	}, nil
}

func managementTenantIdentifierFromContext(ginContext *gin.Context) (managedTenantIdentifier, bool) {
	tenantIdentifier, identifierError := newManagedTenantIdentifier(ginContext.Param("tenant_id"))
	if identifierError != nil {
		ginContext.AbortWithStatus(http.StatusNotFound)
		return "", false
	}
	return tenantIdentifier, true
}

func writeManagementStoreError(ginContext *gin.Context, storeError error) {
	switch {
	case errors.Is(storeError, errManagedTenantNotFound):
		ginContext.AbortWithStatus(http.StatusNotFound)
	case errors.Is(storeError, errManagedTenantNameConflict), errors.Is(storeError, errManagedFinalTenantDeletion), errors.Is(storeError, errManagedProviderKeyConflict):
		ginContext.String(http.StatusConflict, storeError.Error())
	case errors.Is(storeError, errManagedTenantNameInvalid), errors.Is(storeError, errManagedProviderKeyInvalid), errors.Is(storeError, errManagedProviderBaseURLInvalid):
		ginContext.String(http.StatusBadRequest, storeError.Error())
	default:
		ginContext.String(http.StatusInternalServerError, storeError.Error())
	}
}

func writeProviderKeyVerificationError(ginContext *gin.Context, verificationError error) {
	switch {
	case errors.Is(verificationError, errProviderKeyRejected):
		ginContext.String(http.StatusUnprocessableEntity, errProviderKeyRejected.Error())
	case errors.Is(verificationError, errProviderKeyVerificationRateLimited):
		ginContext.String(http.StatusTooManyRequests, errProviderKeyVerificationRateLimited.Error())
	case errors.Is(verificationError, errProviderKeyVerificationTimedOut):
		ginContext.String(http.StatusGatewayTimeout, errProviderKeyVerificationTimedOut.Error())
	default:
		ginContext.String(http.StatusServiceUnavailable, errProviderKeyVerificationUnavailable.Error())
	}
}

func (service *managementService) providerResponses(providerSettings map[providerID]managedProviderSettings) []managementProviderResponse {
	summaries := service.providers.providerSummaries()
	responses := make([]managementProviderResponse, 0, len(summaries))
	for _, summary := range summaries {
		providerIdentifier := providerID(summary.identifier)
		settings, hasKey := providerSettings[providerIdentifier]
		textModels := make([]managementTextModelResponse, 0, len(summary.textModels))
		for _, model := range summary.textModels {
			textModels = append(textModels, managementTextModelResponse{
				ID:              model.identifier,
				ReasoningEffort: managementReasoningEffortCapabilityResponseFor(model.reasoningEffort),
			})
		}
		response := managementProviderResponse{
			ID:                    summary.identifier,
			Label:                 summary.label,
			Aliases:               append([]string{}, summary.aliases...),
			HasKey:                hasKey,
			BaseURL:               constants.EmptyString,
			TextModel:             summary.textDefaultModel,
			SystemPrompt:          constants.EmptyString,
			TextDefaultModel:      summary.textDefaultModel,
			TextModels:            textModels,
			SupportsDictation:     summary.supportsDictation,
			DictationDefaultModel: summary.dictationDefaultModel,
			DictationModels:       summary.dictationModels,
		}
		if hasKey {
			response.MaskedKey = maskedAPIKey(settings.apiKey)
			response.BaseURL = settings.baseURL
			response.TextModel = settings.textModel
			response.SystemPrompt = settings.systemPrompt
		}
		responses = append(responses, response)
	}
	return responses
}

func (request managementDefaultsRequest) tenantDefaults() (TenantDefaults, error) {
	if request.ReasoningEffort == nil {
		return TenantDefaults{}, fmt.Errorf("%w: field=reasoning_effort", errManagementBadRequest)
	}
	return TenantDefaults{
		Provider:          request.Provider,
		Model:             request.Model,
		DictationProvider: request.DictationProvider,
		DictationModel:    request.DictationModel,
		SystemPrompt:      request.SystemPrompt,
		ReasoningEffort:   *request.ReasoningEffort,
	}, nil
}

func managementReasoningEffortCapabilityResponseFor(capability *reasoningEffortCapability) *managementReasoningEffortCapabilityResponse {
	if capability == nil {
		return nil
	}
	return &managementReasoningEffortCapabilityResponse{
		Adapter: string(capability.adapter),
		Efforts: append([]string(nil), capability.efforts...),
	}
}

func (service *managementService) resolveManagedProviderSettings(providerIdentifier providerID, request managementProviderKeyRequest) (providerDefinition, textModelDefinition, error) {
	baseURL, baseURLError := managedProviderBaseURL(providerIdentifier, request.BaseURL)
	if baseURLError != nil {
		return providerDefinition{}, textModelDefinition{}, fmt.Errorf("%w: provider=%s field=base_url", errManagementBadRequest, providerIdentifier.string())
	}
	textModel := strings.TrimSpace(request.TextModel)
	if textModel == constants.EmptyString {
		return providerDefinition{}, textModelDefinition{}, fmt.Errorf("%w: provider=%s field=text_model", errManagementBadRequest, providerIdentifier.string())
	}
	provider, resolvedTextModel, validationError := service.providers.resolveTextModel(providerIdentifier.string(), textModel, providerIdentifier.string(), textModel, false)
	if validationError != nil {
		return providerDefinition{}, textModelDefinition{}, fmt.Errorf("%w: %v", errManagementDefaults, validationError)
	}
	if baseURL != constants.EmptyString {
		provider.textBaseURL = baseURL
	}
	return provider, resolvedTextModel, nil
}

func (service *managementService) validateManagedRoutingDefaults(providerSettings map[providerID]managedProviderSettings, defaults managedRoutingDefaults) error {
	if _, validationError := validatePersistedManagedRoutingDefaults(service.providers, providerSettings, defaults.value()); validationError != nil {
		return fmt.Errorf("%w: %v", errManagementDefaults, validationError)
	}
	return nil
}

func managementMethodUnsafe(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

func managementRequestJSON(rawContentType string) bool {
	contentType := strings.TrimSpace(strings.ToLower(rawContentType))
	if contentType == constants.EmptyString {
		return false
	}
	mediaType, _, _ := strings.Cut(contentType, ";")
	return strings.TrimSpace(mediaType) == mimeApplicationJSON
}

func decodeManagementJSON(ginContext *gin.Context, target any) error {
	jsonDecoder := json.NewDecoder(ginContext.Request.Body)
	jsonDecoder.DisallowUnknownFields()
	if decodeError := jsonDecoder.Decode(target); decodeError != nil {
		return fmt.Errorf("%w: %v", errManagementBadRequest, decodeError)
	}
	return nil
}

func managementPrincipalFromContext(ginContext *gin.Context) managementPrincipal {
	return ginContext.MustGet(contextKeyManagementPrincipal).(managementPrincipal)
}

func managementDefaultsResponse(defaults managedRoutingDefaults) managementTenantDefaultsResponse {
	return managementTenantDefaultsResponse(defaults.value())
}

func managementUsageSummary(summary managedUsageSummary) managementUsageSummaryResponse {
	return managementUsageSummaryResponse{
		Interval:    string(summary.interval),
		BucketUnit:  string(summary.bucketUnit),
		Totals:      managementUsageAggregate(summary.totals),
		Buckets:     managementUsageBuckets(summary.buckets),
		Providers:   managementUsageProviders(summary.providers),
		Models:      managementUsageModels(summary.models),
		StatusCodes: managementUsageStatuses(summary.statusCodes),
	}
}

func managementUsageFailuresResponseFor(page managedUsageFailurePage) managementUsageFailuresResponse {
	failures := make([]managementUsageFailureResponse, 0, len(page.failures))
	for _, failure := range page.failures {
		failures = append(failures, managementUsageFailureResponseFor(failure))
	}
	return managementUsageFailuresResponse{
		Interval:   string(page.interval),
		Failures:   failures,
		NextCursor: page.nextCursor,
	}
}

func managementAccountUsageFailuresResponseFor(page managedUsageFailurePage) managementAccountUsageFailuresResponse {
	failures := make([]managementAccountUsageFailureResponse, 0, len(page.failures))
	for _, failure := range page.failures {
		failures = append(failures, managementAccountUsageFailureResponse{
			TenantID:                       failure.tenantIdentifier,
			TenantName:                     failure.tenantName,
			managementUsageFailureResponse: managementUsageFailureResponseFor(failure),
		})
	}
	return managementAccountUsageFailuresResponse{
		Interval:   string(page.interval),
		Failures:   failures,
		NextCursor: page.nextCursor,
	}
}

func managementUsageFailureResponseFor(failure managedUsageFailure) managementUsageFailureResponse {
	return managementUsageFailureResponse{
		OccurredAt:          failure.occurredAt.UTC().Format(time.RFC3339Nano),
		Endpoint:            failure.endpoint,
		Provider:            failure.providerIdentifier,
		Model:               failure.modelIdentifier,
		StatusCode:          failure.statusCode,
		OutcomeCode:         string(failure.outcomeCode),
		LatencyMilliseconds: failure.latencyMilliseconds,
	}
}

func managementAdminUsageSummary(summary managedAdminUsageSummary) managementAdminUsageSummaryResponse {
	return managementAdminUsageSummaryResponse{
		PeriodDays:  summary.periodDays,
		Totals:      managementUsageAggregate(summary.totals),
		Daily:       managementUsageDaily(summary.daily),
		Providers:   managementUsageProviders(summary.providers),
		Models:      managementUsageModels(summary.models),
		StatusCodes: managementUsageStatuses(summary.statusCodes),
	}
}

func managementUsageAggregate(aggregate managedUsageAggregate) managementUsageAggregateResponse {
	return managementUsageAggregateResponse{
		Requests:                   aggregate.requests,
		SuccessfulRequests:         aggregate.successfulRequests,
		FailedRequests:             aggregate.failedRequests,
		TextRequests:               aggregate.textRequests,
		DictationRequests:          aggregate.dictationRequests,
		RequestTokens:              aggregate.requestTokens,
		ResponseTokens:             aggregate.responseTokens,
		TotalTokens:                aggregate.totalTokens,
		AverageLatencyMilliseconds: aggregate.averageLatencyMillis,
	}
}

func managementUsageDaily(daily []managedUsageDailyBucket) []managementUsageDailyResponse {
	responses := make([]managementUsageDailyResponse, 0, len(daily))
	for _, bucket := range daily {
		responses = append(responses, managementUsageDailyResponse{
			Date: bucket.date,
			Data: managementUsageAggregate(bucket.aggregate),
		})
	}
	return responses
}

func managementUsageBuckets(buckets []managedUsageBucket) []managementUsageBucketResponse {
	responses := make([]managementUsageBucketResponse, 0, len(buckets))
	for _, bucket := range buckets {
		responses = append(responses, managementUsageBucketResponse{
			Start: bucket.start.UTC().Format(time.RFC3339Nano),
			Data:  managementUsageAggregate(bucket.aggregate),
		})
	}
	return responses
}

func managementUsageProviders(providers []managedUsageProviderBucket) []managementUsageProviderResponse {
	responses := make([]managementUsageProviderResponse, 0, len(providers))
	for _, bucket := range providers {
		responses = append(responses, managementUsageProviderResponse{
			Provider: bucket.providerIdentifier,
			Data:     managementUsageAggregate(bucket.aggregate),
		})
	}
	return responses
}

func managementUsageModels(models []managedUsageModelBucket) []managementUsageModelResponse {
	responses := make([]managementUsageModelResponse, 0, len(models))
	for _, bucket := range models {
		responses = append(responses, managementUsageModelResponse{
			Provider: bucket.providerIdentifier,
			Model:    bucket.modelIdentifier,
			Data:     managementUsageAggregate(bucket.aggregate),
		})
	}
	return responses
}

func managementUsageStatuses(statusCodes []managedUsageStatusBucket) []managementUsageStatusResponse {
	responses := make([]managementUsageStatusResponse, 0, len(statusCodes))
	for _, bucket := range statusCodes {
		responses = append(responses, managementUsageStatusResponse{
			StatusCode: bucket.statusCode,
			Requests:   bucket.requests,
		})
	}
	return responses
}

func (service *managementService) adminUsersResponse(snapshots []managedAdminUserSnapshot) managementAdminUsersResponse {
	users := make([]managementAdminUserResponse, 0, len(snapshots))
	for _, snapshot := range snapshots {
		_, userIsAdmin := service.sessionValidator.adminEmails[strings.ToLower(strings.TrimSpace(snapshot.userEmail))]
		tenants := make([]managementAdminTenantResponse, 0, len(snapshot.tenants))
		for _, tenantSnapshot := range snapshot.tenants {
			tenants = append(tenants, managementAdminTenantResponse{
				ID:        tenantSnapshot.tenantID,
				Name:      tenantSnapshot.name,
				HasSecret: tenantSnapshot.hasSecret,
				CreatedAt: tenantSnapshot.createdAt.Format(time.RFC3339),
				UpdatedAt: tenantSnapshot.updatedAt.Format(time.RFC3339),
				Usage:     managementAdminUsageSummary(tenantSnapshot.usage),
			})
		}
		users = append(users, managementAdminUserResponse{
			User: managementUserResponse{
				ID:          snapshot.userID,
				Email:       snapshot.userEmail,
				DisplayName: snapshot.userDisplayName,
				AvatarURL:   snapshot.userAvatarURL,
				IsAdmin:     userIsAdmin,
			},
			TenantCount: len(tenants),
			Tenants:     tenants,
		})
	}
	return managementAdminUsersResponse{
		PeriodDays: managedUsageSummaryDays,
		Users:      users,
	}
}
