package proxy

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/tyemirov/tauth/pkg/sessionvalidator"
)

var (
	errManagementSessionInvalidPrincipal = errors.New("management_session_invalid_principal")
	errManagementSessionWrongTenant      = errors.New("management_session_wrong_tenant")
)

type managementSessionValidator struct {
	tauth       *sessionvalidator.Validator
	tenantID    string
	adminEmails map[string]struct{}
}

type managementPrincipal struct {
	userID          string
	userEmail       string
	userDisplayName string
	userAvatarURL   string
	tenantID        string
	isAdmin         bool
}

func newManagementSessionValidator(configuration ManagementConfiguration) (*managementSessionValidator, error) {
	if strings.TrimSpace(configuration.SessionCookieName) == "" {
		return nil, fmt.Errorf("%w: field=management.session_cookie_name", ErrInvalidManagementConfiguration)
	}
	tauthValidator, validatorError := sessionvalidator.New(sessionvalidator.Config{
		SigningKey: []byte(configuration.JWTSigningKey),
		Issuer:     configuration.JWTIssuer,
		CookieName: configuration.SessionCookieName,
	})
	if validatorError != nil {
		return nil, fmt.Errorf("management_session.new: %w", validatorError)
	}
	return &managementSessionValidator{
		tauth:       tauthValidator,
		tenantID:    configuration.TAuthTenantID,
		adminEmails: managementAdminEmailSet(configuration.AdminEmails),
	}, nil
}

func (validator *managementSessionValidator) validateRequest(request *http.Request) (managementPrincipal, error) {
	claims, validationError := validator.tauth.ValidateRequest(request)
	if validationError != nil {
		return managementPrincipal{}, fmt.Errorf("management_session.validate_request: %w", validationError)
	}
	if claims.ExpiresAt == nil {
		return managementPrincipal{}, fmt.Errorf("management_session.validate_request: %w", errManagementSessionInvalidPrincipal)
	}
	if strings.TrimSpace(claims.TenantID) != validator.tenantID {
		return managementPrincipal{}, fmt.Errorf("management_session.validate_request: %w", errManagementSessionWrongTenant)
	}
	return validator.newManagementPrincipal(claims)
}

func (validator *managementSessionValidator) newManagementPrincipal(claims *sessionvalidator.Claims) (managementPrincipal, error) {
	userID := strings.TrimSpace(claims.UserID)
	if userID == "" {
		return managementPrincipal{}, fmt.Errorf("management_session.principal: %w", errManagementSessionInvalidPrincipal)
	}
	userEmail := strings.ToLower(strings.TrimSpace(claims.UserEmail))
	_, userIsAdmin := validator.adminEmails[userEmail]
	return managementPrincipal{
		userID:          userID,
		userEmail:       userEmail,
		userDisplayName: strings.TrimSpace(claims.UserDisplayName),
		userAvatarURL:   strings.TrimSpace(claims.UserAvatarURL),
		tenantID:        strings.TrimSpace(claims.TenantID),
		isAdmin:         userIsAdmin,
	}, nil
}

func managementSessionRejectionReason(validationError error) string {
	switch {
	case errors.Is(validationError, sessionvalidator.ErrMissingCookie):
		return "missing_cookie"
	case errors.Is(validationError, sessionvalidator.ErrTokenExpired):
		return "expired"
	case errors.Is(validationError, sessionvalidator.ErrInvalidIssuer):
		return "invalid_issuer"
	case errors.Is(validationError, errManagementSessionWrongTenant):
		return "wrong_tenant"
	default:
		return "invalid"
	}
}

func managementAdminEmailSet(adminEmails []string) map[string]struct{} {
	adminEmailSet := make(map[string]struct{}, len(adminEmails))
	for _, emailValue := range adminEmails {
		normalizedEmail, emailError := normalizeManagementEmail(emailValue)
		if emailError == nil {
			adminEmailSet[normalizedEmail] = struct{}{}
		}
	}
	return adminEmailSet
}
