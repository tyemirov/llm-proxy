package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tyemirov/tauth/pkg/oauthvalidator"
	"gorm.io/gorm"
)

const (
	mcpPath             = "/mcp"
	mcpMetadataPath     = "/.well-known/oauth-protected-resource/mcp"
	mcpProtocolVersion  = "2026-07-28"
	mcpUseScope         = "llm-proxy:use"
	mcpListTenantsTool  = "llm_proxy.list_tenants"
	mcpGenerateTextTool = "llm_proxy.generate_text"
	mcpRoutesTemplate   = "llm-proxy://tenants/{tenant_id}/routes"
	mcpNotFound         = "not_found"
)

type mcpIdentityKey struct{}

type mcpIdentity struct {
	subject   string
	requestID string
}

type mcpTenantList struct {
	Tenants []managementTenantSummaryResponse `json:"tenants"`
}

type mcpResourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	ScopesSupported        []string `json:"scopes_supported"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
}

func registerMCPRoutes(router *gin.Engine, configuration Configuration, service *managementService, upstream *providerRouter, assets *tenantAssetStore) error {
	issuer := configuration.Management.TAuthURL
	audience := configuration.Management.ProxyOrigin
	validator, err := oauthvalidator.New(oauthvalidator.Config{
		Issuer: issuer, Audience: audience, JWKSURL: issuer + "/oauth/jwks", RequiredScopes: []string{mcpUseScope},
	})
	if err != nil {
		return fmt.Errorf("mcp.configure: %w", err)
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "llm-proxy", Version: "1.0.0"}, &mcp.ServerOptions{
		Instructions: "Use llm_proxy.list_tenants, then select an owned tenant_id for generation and route discovery.",
		Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}, Resources: &mcp.ResourceCapabilities{}},
	})
	server.AddReceivingMiddleware(mcpCurrentProtocol)
	// This fixed DTO has no custom JSON marshaler or unsupported schema types.
	listSchema, _ := jsonschema.For[mcpTenantList](nil)
	mcp.AddTool[struct{}, any](server, &mcp.Tool{OutputSchema: listSchema, Name: mcpListTenantsTool, Description: "Read all tenants owned by this account.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(false)}},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			identity := ctx.Value(mcpIdentityKey{}).(mcpIdentity)
			summaries, err := service.store.ownedTenantSummaries(identity.subject)
			if err != nil {
				return mcpToolFailure(string(managedUsageOutcomeProxyError)), nil, nil
			}
			return nil, mcpTenantList{Tenants: tenantSummaryResponses(summaries)}, nil
		})
	registerMCPGeneration(server, configuration, service, upstream, assets)
	server.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: mcpRoutesTemplate, Name: "Tenant text routes", MIMEType: mimeApplicationJSON},
		func(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			// The SDK matched the complete URI against the registered template.
			identifier := strings.TrimSuffix(strings.TrimPrefix(request.Params.URI, "llm-proxy://tenants/"), "/routes")
			identity := ctx.Value(mcpIdentityKey{}).(mcpIdentity)
			requestTenant, err := service.store.mcpTenant(identity.subject, identifier)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, mcpResourceNotFound()
			}
			if err != nil {
				return nil, &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: "Tenant routes unavailable"}
			}
			// The route DTO contains only strings, booleans, and slices.
			body, _ := json.Marshal(mcpRoutesFor(requestTenant, service.providers))
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: request.Params.URI, MIMEType: mimeApplicationJSON, Text: string(body)}}}, nil
		})
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, PropagateRequestCancellation: true,
		MaxRequestBodyBytes: maximumV2RequestBytes(configuration.MaxPromptBytes, configuration.ModelCatalog),
	})
	router.GET(mcpMetadataPath, func(c *gin.Context) {
		c.Header(headerCacheControl, cacheControlNoStore)
		c.JSON(http.StatusOK, mcpResourceMetadata{Resource: audience, AuthorizationServers: []string{issuer}, ScopesSupported: []string{mcpUseScope}, BearerMethodsSupported: []string{"header"}})
	})
	router.POST(mcpPath, func(c *gin.Context) {
		c.Header(headerCacheControl, cacheControlNoStore)
		challenge := fmt.Sprintf(`Bearer resource_metadata=%q, scope=%q`, audience+mcpMetadataPath, mcpUseScope)
		claims, validationError := validator.ValidateRequest(c.Request)
		if errors.Is(validationError, oauthvalidator.ErrInsufficientScope) {
			c.Header("WWW-Authenticate", challenge+`, error="insufficient_scope"`)
			c.Status(http.StatusForbidden)
			return
		}
		if validationError != nil || claims.TenantID != configuration.Management.TAuthTenantID || len(c.Request.Header.Values("Authorization")) != 1 {
			c.Header("WWW-Authenticate", challenge)
			c.Status(http.StatusUnauthorized)
			return
		}

		if version := c.GetHeader("MCP-Protocol-Version"); version != mcpProtocolVersion || c.Request.URL.RawQuery != "" || c.GetHeader("Mcp-Session-Id") != "" {
			c.Status(http.StatusBadRequest)
			return
		}
		if origin := c.GetHeader("Origin"); origin != "" && origin != audience {
			c.Status(http.StatusForbidden)
			return
		}
		identity := mcpIdentity{subject: claims.Subject, requestID: requestIDFromContext(c)}
		transport.ServeHTTP(c.Writer, c.Request.WithContext(context.WithValue(c.Request.Context(), mcpIdentityKey{}, identity)))
	})
	return nil
}

func mcpCurrentProtocol(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
		result, err := next(ctx, method, request)
		if toolResult, ok := result.(*mcp.CallToolResult); ok && toolResult.IsError && toolResult.StructuredContent == nil {
			return mcpToolFailure(string(managedUsageOutcomeInvalidRequest)), nil
		}
		if discovery, ok := result.(*mcp.DiscoverResult); ok {
			discovery.SupportedVersions = []string{mcpProtocolVersion}
		}
		return result, err
	}
}

func mcpToolFailure(code string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: code}}, StructuredContent: struct {
		Code string `json:"code"`
	}{code}}
}

func mcpResourceNotFound() error {
	return &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "Tenant resource not found"}
}

func (store *managedTenantStore) ownedTenantSummaries(subject string) ([]managedTenantSummary, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, err := store.database.userByID(subject)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return []managedTenantSummary{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("mcp.tenants.read: %w", err)
	}
	return managedAccountSnapshotFor(record).tenants, nil
}

func (store *managedTenantStore) mcpTenant(subject, identifier string) (tenant, error) {
	if identifier == "" || strings.TrimSpace(identifier) != identifier {
		return tenant{}, gorm.ErrRecordNotFound
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, err := store.database.tenantByOwnerAndID(subject, identifier)
	if err != nil {
		return tenant{}, fmt.Errorf("mcp.tenant.read: %w", err)
	}
	return store.tenant(record, [sha256.Size]byte{})
}
