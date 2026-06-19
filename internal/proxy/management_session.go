package proxy

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	errManagementSessionMissingCookie = errors.New("management_session_missing_cookie")
	errManagementSessionInvalid       = errors.New("management_session_invalid")
	errManagementSessionWrongTenant   = errors.New("management_session_wrong_tenant")
)

type managementSessionValidator struct {
	signingKey []byte
	issuer     string
	cookieName string
	tenantID   string
	now        func() time.Time
}

type managementSessionClaims struct {
	TenantID        string   `json:"tenant_id"`
	UserID          string   `json:"user_id"`
	UserEmail       string   `json:"user_email"`
	UserDisplayName string   `json:"user_display_name"`
	UserAvatarURL   string   `json:"user_avatar_url"`
	UserRoles       []string `json:"user_roles"`
	jwt.RegisteredClaims
}

type managementPrincipal struct {
	userID          string
	userEmail       string
	userDisplayName string
	userAvatarURL   string
	tenantID        string
}

func newManagementSessionValidator(configuration ManagementConfiguration) *managementSessionValidator {
	return &managementSessionValidator{
		signingKey: []byte(configuration.JWTSigningKey),
		issuer:     configuration.JWTIssuer,
		cookieName: configuration.SessionCookieName,
		tenantID:   configuration.TAuthTenantID,
		now:        time.Now,
	}
}

func (validator *managementSessionValidator) validateRequest(request *http.Request) (managementPrincipal, error) {
	sessionCookie, cookieError := request.Cookie(validator.cookieName)
	if cookieError != nil || sessionCookie == nil || strings.TrimSpace(sessionCookie.Value) == "" {
		return managementPrincipal{}, fmt.Errorf("management_session.validate_request: %w", errManagementSessionMissingCookie)
	}
	return validator.validateToken(sessionCookie.Value)
}

func (validator *managementSessionValidator) validateToken(rawToken string) (managementPrincipal, error) {
	parsedToken, parseError := jwt.ParseWithClaims(rawToken, &managementSessionClaims{}, func(parsedToken *jwt.Token) (interface{}, error) {
		return validator.signingKey, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithTimeFunc(func() time.Time {
		return validator.now().UTC()
	}))
	if parseError != nil || parsedToken == nil || !parsedToken.Valid {
		return managementPrincipal{}, fmt.Errorf("management_session.validate_token: %w", errManagementSessionInvalid)
	}
	claims := parsedToken.Claims.(*managementSessionClaims)
	if claims.Issuer != validator.issuer {
		return managementPrincipal{}, fmt.Errorf("management_session.validate_token: %w", errManagementSessionInvalid)
	}
	if claims.IssuedAt != nil && validator.now().UTC().Before(claims.IssuedAt.Time) {
		return managementPrincipal{}, fmt.Errorf("management_session.validate_token: %w", errManagementSessionInvalid)
	}
	if strings.TrimSpace(claims.TenantID) != validator.tenantID {
		return managementPrincipal{}, fmt.Errorf("management_session.validate_token: %w", errManagementSessionWrongTenant)
	}
	return newManagementPrincipal(claims)
}

func newManagementPrincipal(claims *managementSessionClaims) (managementPrincipal, error) {
	userID := strings.TrimSpace(claims.UserID)
	if userID == "" {
		return managementPrincipal{}, fmt.Errorf("management_session.principal: %w", errManagementSessionInvalid)
	}
	return managementPrincipal{
		userID:          userID,
		userEmail:       strings.TrimSpace(claims.UserEmail),
		userDisplayName: strings.TrimSpace(claims.UserDisplayName),
		userAvatarURL:   strings.TrimSpace(claims.UserAvatarURL),
		tenantID:        strings.TrimSpace(claims.TenantID),
	}, nil
}
