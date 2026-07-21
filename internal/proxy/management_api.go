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
	managementProfilePath               = "/profile"
	managementProviderKeysPath          = "/provider-keys/:provider"
	managementDefaultsPath              = "/defaults"
	managementSecretsPath               = "/secrets"
	managementUsagePath                 = "/usage"
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
	authenticator    tenantAuthenticator
	structuredLogger *zap.SugaredLogger
}

type managementProfileResponse struct {
	User      managementUserResponse       `json:"user"`
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
	HasSecret bool                             `json:"has_secret"`
	Defaults  managementTenantDefaultsResponse `json:"defaults"`
	CreatedAt string                           `json:"created_at"`
	UpdatedAt string                           `json:"updated_at"`
}

type managementTenantDefaultsResponse struct {
	Provider          string `json:"provider"`
	Model             string `json:"model"`
	DictationProvider string `json:"dictation_provider"`
	DictationModel    string `json:"dictation_model"`
	SystemPrompt      string `json:"system_prompt"`
}

type managementProviderResponse struct {
	ID                    string   `json:"id"`
	Label                 string   `json:"label"`
	Aliases               []string `json:"aliases"`
	HasKey                bool     `json:"has_key"`
	MaskedKey             string   `json:"masked_key,omitempty"`
	TextModel             string   `json:"text_model"`
	SystemPrompt          string   `json:"system_prompt"`
	TextDefaultModel      string   `json:"text_default_model"`
	TextModels            []string `json:"text_models"`
	SupportsDictation     bool     `json:"supports_dictation"`
	DictationDefaultModel string   `json:"dictation_default_model,omitempty"`
	DictationModels       []string `json:"dictation_models"`
}

type managementProxyResponse struct {
	TextPath      string `json:"text_path"`
	V2Path        string `json:"v2_path"`
	DictationPath string `json:"dictation_path"`
}

type managementSecretResponse struct {
	Secret  string                    `json:"secret"`
	Profile managementProfileResponse `json:"profile"`
}

type managementUsageSummaryResponse struct {
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
	User   managementUserResponse         `json:"user"`
	Tenant managementAdminTenantResponse  `json:"tenant"`
	Usage  managementUsageSummaryResponse `json:"usage"`
}

type managementAdminTenantResponse struct {
	ID        string `json:"id"`
	HasSecret bool   `json:"has_secret"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type managementProviderKeyRequest struct {
	APIKey       string `json:"api_key"`
	TextModel    string `json:"text_model"`
	SystemPrompt string `json:"system_prompt"`
}

type managementDefaultsRequest struct {
	Provider          string `json:"provider"`
	Model             string `json:"model"`
	DictationProvider string `json:"dictation_provider"`
	DictationModel    string `json:"dictation_model"`
	SystemPrompt      string `json:"system_prompt"`
}

func newManagementService(configuration ManagementConfiguration, sessionValidator *managementSessionValidator, store *managedTenantStore, providers *providerRegistry, authenticator tenantAuthenticator, structuredLogger *zap.SugaredLogger) *managementService {
	return &managementService{
		configuration:    configuration,
		sessionValidator: sessionValidator,
		store:            store,
		providers:        providers,
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
	managementGroup.GET(managementProfilePath, service.profileHandler())
	managementGroup.GET(managementUsagePath, service.usageHandler())
	managementGroup.GET(managementAdminUsersPath, service.adminUsersHandler())
	managementGroup.PUT(managementProviderKeysPath, service.saveProviderKeyHandler())
	managementGroup.DELETE(managementProviderKeysPath, service.removeProviderKeyHandler())
	managementGroup.PUT(managementDefaultsPath, service.updateDefaultsHandler())
	managementGroup.POST(managementSecretsPath, service.generateSecretHandler())
	managementGroup.DELETE(managementSecretsPath, service.revokeSecretHandler())
}

func (service *managementService) sessionMiddleware() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		principal, validationError := service.sessionValidator.validateRequest(ginContext.Request)
		if validationError != nil {
			service.structuredLogger.Warnw("management session rejected", "reason", managementSessionRejectionReason(validationError))
			ginContext.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if migrationError := service.store.claimLegacyToken(principal); migrationError != nil {
			statusCode := http.StatusInternalServerError
			if errors.Is(migrationError, errManagedLegacyTokenConflict) {
				statusCode = http.StatusConflict
			}
			ginContext.String(statusCode, migrationError.Error())
			ginContext.Abort()
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

func (service *managementService) profileHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		principal := managementPrincipalFromContext(ginContext)
		snapshot, snapshotError := service.store.profile(principal)
		if snapshotError != nil {
			ginContext.String(http.StatusInternalServerError, snapshotError.Error())
			return
		}
		service.writeProfileResponse(ginContext, principal, snapshot)
	}
}

func (service *managementService) usageHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		principal := managementPrincipalFromContext(ginContext)
		summary, summaryError := service.store.usageSummary(principal)
		if summaryError != nil {
			ginContext.String(http.StatusInternalServerError, summaryError.Error())
			return
		}
		ginContext.JSON(http.StatusOK, managementUsageSummary(summary))
	}
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
		if providerSettingsError := service.validateManagedProviderSettings(providerIdentifier, request); providerSettingsError != nil {
			ginContext.String(http.StatusBadRequest, providerSettingsError.Error())
			return
		}
		snapshot, storeError := service.store.saveProviderKey(principal, providerIdentifier, request.APIKey, request.TextModel, request.SystemPrompt)
		if storeError != nil {
			ginContext.String(http.StatusBadRequest, storeError.Error())
			return
		}
		service.writeProfileResponse(ginContext, principal, snapshot)
	}
}

func (service *managementService) removeProviderKeyHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		principal := managementPrincipalFromContext(ginContext)
		providerIdentifier, providerError := service.providers.canonicalProviderID(ginContext.Param("provider"))
		if providerError != nil {
			ginContext.String(http.StatusBadRequest, providerError.Error())
			return
		}
		snapshot, storeError := service.store.removeProviderKey(principal, providerIdentifier)
		if storeError != nil {
			ginContext.String(http.StatusInternalServerError, storeError.Error())
			return
		}
		service.writeProfileResponse(ginContext, principal, snapshot)
	}
}

func (service *managementService) updateDefaultsHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		principal := managementPrincipalFromContext(ginContext)
		var request managementDefaultsRequest
		if decodeError := decodeManagementJSON(ginContext, &request); decodeError != nil {
			ginContext.String(http.StatusBadRequest, decodeError.Error())
			return
		}
		defaults, defaultsConstructionError := newManagedRoutingDefaults(service.providers, TenantDefaults(request))
		if defaultsConstructionError != nil {
			ginContext.String(http.StatusBadRequest, defaultsConstructionError.Error())
			return
		}
		currentSnapshot, snapshotError := service.store.profile(principal)
		if snapshotError != nil {
			ginContext.String(http.StatusInternalServerError, snapshotError.Error())
			return
		}
		if defaultsError := service.validateManagedRoutingDefaults(currentSnapshot.providerAPIKeys, defaults); defaultsError != nil {
			ginContext.String(http.StatusBadRequest, defaultsError.Error())
			return
		}
		snapshot, storeError := service.store.updateDefaults(principal, defaults)
		if storeError != nil {
			ginContext.String(http.StatusInternalServerError, storeError.Error())
			return
		}
		service.writeProfileResponse(ginContext, principal, snapshot)
	}
}

func (service *managementService) generateSecretHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		principal := managementPrincipalFromContext(ginContext)
		rawSecret, snapshot, generationError := service.store.generateSecret(principal, service.authenticator.containsStaticSecretDigest)
		if generationError != nil {
			ginContext.String(http.StatusInternalServerError, generationError.Error())
			return
		}
		profile, profileError := service.profileResponse(principal, snapshot)
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

func (service *managementService) revokeSecretHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		principal := managementPrincipalFromContext(ginContext)
		snapshot, storeError := service.store.revokeSecret(principal)
		if storeError != nil {
			ginContext.String(http.StatusInternalServerError, storeError.Error())
			return
		}
		service.writeProfileResponse(ginContext, principal, snapshot)
	}
}

func (service *managementService) writeProfileResponse(ginContext *gin.Context, principal managementPrincipal, snapshot managedTenantSnapshot) {
	profile, profileError := service.profileResponse(principal, snapshot)
	if profileError != nil {
		ginContext.String(http.StatusInternalServerError, profileError.Error())
		return
	}
	ginContext.Header(headerCacheControl, cacheControlNoStore)
	ginContext.JSON(http.StatusOK, profile)
}

func (service *managementService) profileResponse(principal managementPrincipal, snapshot managedTenantSnapshot) (managementProfileResponse, error) {
	defaults, defaultsError := validatePersistedManagedRoutingDefaults(service.providers, snapshot.defaults)
	if defaultsError != nil {
		return managementProfileResponse{}, fmt.Errorf("%w: tenant=%s: %w", errManagedRoutingDefaultsInvalid, snapshot.tenantID, defaultsError)
	}
	return managementProfileResponse{
		User: managementUserResponse{
			ID:          snapshot.userID,
			Email:       snapshot.userEmail,
			DisplayName: snapshot.userDisplayName,
			AvatarURL:   snapshot.userAvatarURL,
			IsAdmin:     principal.isAdmin,
		},
		Tenant: managementTenantResponse{
			ID:        snapshot.tenantID,
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

func (service *managementService) providerResponses(providerSettings map[providerID]managedProviderSettings) []managementProviderResponse {
	summaries := service.providers.providerSummaries()
	responses := make([]managementProviderResponse, 0, len(summaries))
	for _, summary := range summaries {
		providerIdentifier := providerID(summary.identifier)
		settings, hasKey := providerSettings[providerIdentifier]
		response := managementProviderResponse{
			ID:                    summary.identifier,
			Label:                 summary.label,
			Aliases:               summary.aliases,
			HasKey:                hasKey,
			TextModel:             summary.textDefaultModel,
			SystemPrompt:          constants.EmptyString,
			TextDefaultModel:      summary.textDefaultModel,
			TextModels:            summary.textModels,
			SupportsDictation:     summary.supportsDictation,
			DictationDefaultModel: summary.dictationDefaultModel,
			DictationModels:       summary.dictationModels,
		}
		if hasKey {
			response.MaskedKey = maskedAPIKey(settings.apiKey)
			response.TextModel = settings.textModel
			response.SystemPrompt = settings.systemPrompt
		}
		responses = append(responses, response)
	}
	return responses
}

func (service *managementService) validateManagedProviderSettings(providerIdentifier providerID, request managementProviderKeyRequest) error {
	textModel := strings.TrimSpace(request.TextModel)
	if textModel == constants.EmptyString {
		return fmt.Errorf("%w: provider=%s field=text_model", errManagementBadRequest, providerIdentifier.string())
	}
	if _, _, validationError := service.providers.resolveTextModel(providerIdentifier.string(), textModel, providerIdentifier.string(), textModel, false); validationError != nil {
		return fmt.Errorf("%w: %v", errManagementDefaults, validationError)
	}
	return nil
}

func (service *managementService) validateManagedRoutingDefaults(providerAPIKeys map[providerID]string, defaults managedRoutingDefaults) error {
	requestTenant := tenant{
		identifier:      tenantID("management-validation"),
		defaults:        newTenantDefaults(defaults.value()),
		managed:         true,
		providerAPIKeys: providerAPIKeys,
	}
	validator := newModelValidator(service.providers.forTenant(requestTenant))
	if _, _, validationError := validator.ResolveText(constants.EmptyString, constants.EmptyString, requestTenant.defaults.provider, requestTenant.defaults.model, false); validationError != nil {
		return fmt.Errorf("%w: %v", errManagementDefaults, validationError)
	}
	if _, _, validationError := validator.ResolveDictation(constants.EmptyString, constants.EmptyString, requestTenant.defaults.dictationProvider, requestTenant.defaults.dictationModel); validationError != nil {
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
		users = append(users, managementAdminUserResponse{
			User: managementUserResponse{
				ID:          snapshot.userID,
				Email:       snapshot.userEmail,
				DisplayName: snapshot.userDisplayName,
				AvatarURL:   snapshot.userAvatarURL,
				IsAdmin:     userIsAdmin,
			},
			Tenant: managementAdminTenantResponse{
				ID:        snapshot.tenantID,
				HasSecret: snapshot.hasSecret,
				CreatedAt: snapshot.createdAt.Format(time.RFC3339),
				UpdatedAt: snapshot.updatedAt.Format(time.RFC3339),
			},
			Usage: managementUsageSummary(snapshot.usage),
		})
	}
	return managementAdminUsersResponse{
		PeriodDays: managedUsageSummaryDays,
		Users:      users,
	}
}
