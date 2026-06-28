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

type managementProviderKeyRequest struct {
	APIKey string `json:"api_key"`
}

type managementDefaultsRequest struct {
	Provider          string `json:"provider"`
	Model             string `json:"model"`
	DictationProvider string `json:"dictation_provider"`
	DictationModel    string `json:"dictation_model"`
	SystemPrompt      string `json:"system_prompt"`
}

func newManagementService(configuration ManagementConfiguration, store *managedTenantStore, providers *providerRegistry, authenticator tenantAuthenticator, structuredLogger *zap.SugaredLogger) *managementService {
	return &managementService{
		configuration:    configuration,
		sessionValidator: newManagementSessionValidator(configuration),
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
		ginContext.JSON(http.StatusOK, service.profileResponse(snapshot))
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
		snapshot, storeError := service.store.saveProviderKey(principal, providerIdentifier, request.APIKey)
		if storeError != nil {
			ginContext.String(http.StatusBadRequest, storeError.Error())
			return
		}
		ginContext.JSON(http.StatusOK, service.profileResponse(snapshot))
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
		ginContext.JSON(http.StatusOK, service.profileResponse(snapshot))
	}
}

func (service *managementService) updateDefaultsHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		principal := managementPrincipalFromContext(ginContext)
		currentSnapshot, snapshotError := service.store.profile(principal)
		if snapshotError != nil {
			ginContext.String(http.StatusInternalServerError, snapshotError.Error())
			return
		}
		var request managementDefaultsRequest
		if decodeError := decodeManagementJSON(ginContext, &request); decodeError != nil {
			ginContext.String(http.StatusBadRequest, decodeError.Error())
			return
		}
		defaults := TenantDefaults(request)
		if defaultsError := service.validateManagedTextDefaults(currentSnapshot.providerAPIKeys, defaults); defaultsError != nil {
			ginContext.String(http.StatusBadRequest, defaultsError.Error())
			return
		}
		snapshot, storeError := service.store.updateDefaults(principal, defaults)
		if storeError != nil {
			ginContext.String(http.StatusInternalServerError, storeError.Error())
			return
		}
		ginContext.JSON(http.StatusOK, service.profileResponse(snapshot))
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
		ginContext.JSON(http.StatusOK, managementSecretResponse{
			Secret:  rawSecret,
			Profile: service.profileResponse(snapshot),
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
		ginContext.JSON(http.StatusOK, service.profileResponse(snapshot))
	}
}

func (service *managementService) profileResponse(snapshot managedTenantSnapshot) managementProfileResponse {
	return managementProfileResponse{
		User: managementUserResponse{
			ID:          snapshot.userID,
			Email:       snapshot.userEmail,
			DisplayName: snapshot.userDisplayName,
			AvatarURL:   snapshot.userAvatarURL,
		},
		Tenant: managementTenantResponse{
			ID:        snapshot.tenantID,
			HasSecret: snapshot.hasSecret,
			Defaults:  managementDefaultsResponse(snapshot.defaults),
			CreatedAt: snapshot.createdAt.Format(time.RFC3339),
			UpdatedAt: snapshot.updatedAt.Format(time.RFC3339),
		},
		Providers: service.providerResponses(snapshot.providerAPIKeys),
		Proxy: managementProxyResponse{
			TextPath:      rootPath,
			V2Path:        v2Path,
			DictationPath: dictatePath,
		},
	}
}

func (service *managementService) providerResponses(providerAPIKeys map[providerID]string) []managementProviderResponse {
	summaries := service.providers.providerSummaries()
	responses := make([]managementProviderResponse, 0, len(summaries))
	for _, summary := range summaries {
		providerIdentifier := providerID(summary.identifier)
		apiKey, hasKey := providerAPIKeys[providerIdentifier]
		response := managementProviderResponse{
			ID:                    summary.identifier,
			Label:                 summary.label,
			Aliases:               summary.aliases,
			HasKey:                hasKey,
			TextDefaultModel:      summary.textDefaultModel,
			TextModels:            summary.textModels,
			SupportsDictation:     summary.supportsDictation,
			DictationDefaultModel: summary.dictationDefaultModel,
			DictationModels:       summary.dictationModels,
		}
		if hasKey {
			response.MaskedKey = maskedAPIKey(apiKey)
		}
		responses = append(responses, response)
	}
	return responses
}

func (service *managementService) validateManagedTextDefaults(providerAPIKeys map[providerID]string, defaults TenantDefaults) error {
	requestTenant := tenant{
		identifier:      tenantID("management-validation"),
		defaults:        newTenantDefaults(defaults),
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

func managementDefaultsResponse(defaults TenantDefaults) managementTenantDefaultsResponse {
	normalizedDefaults := newTenantDefaults(defaults)
	return managementTenantDefaultsResponse{
		Provider:          normalizedDefaults.provider,
		Model:             normalizedDefaults.model,
		DictationProvider: normalizedDefaults.dictationProvider,
		DictationModel:    normalizedDefaults.dictationModel,
		SystemPrompt:      normalizedDefaults.systemPrompt,
	}
}
