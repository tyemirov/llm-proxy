package proxy

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	upstreamOriginSchemeHTTPS = "https"
	upstreamOriginSchemeHTTP  = "http"
)

// ErrInvalidUpstreamRateLimitConfiguration identifies an invalid shared upstream rate-limit rule.
var ErrInvalidUpstreamRateLimitConfiguration = errors.New("invalid_upstream_rate_limit_configuration")

// UpstreamRateLimitConfiguration describes one exact-origin rolling-window limit.
type UpstreamRateLimitConfiguration struct {
	Origin      string
	MaxRequests int
	Interval    string
}

type upstreamRateLimits struct {
	rules map[string]upstreamRateLimitRule
}

type upstreamRateLimitRule struct {
	maxRequests int
	interval    time.Duration
}

func newUpstreamRateLimits(configurations []UpstreamRateLimitConfiguration) (upstreamRateLimits, error) {
	rules := make(map[string]upstreamRateLimitRule, len(configurations))
	for configurationIndex, configuration := range configurations {
		normalizedOrigin, originError := normalizedUpstreamOrigin(configuration.Origin)
		if originError != nil {
			return upstreamRateLimits{}, fmt.Errorf("%w: rule=%d field=origin", ErrInvalidUpstreamRateLimitConfiguration, configurationIndex)
		}
		if configuration.MaxRequests <= 0 {
			return upstreamRateLimits{}, fmt.Errorf("%w: rule=%d field=max_requests", ErrInvalidUpstreamRateLimitConfiguration, configurationIndex)
		}
		interval, intervalError := time.ParseDuration(strings.TrimSpace(configuration.Interval))
		if intervalError != nil || interval <= 0 {
			return upstreamRateLimits{}, fmt.Errorf("%w: rule=%d field=interval", ErrInvalidUpstreamRateLimitConfiguration, configurationIndex)
		}
		if _, duplicateOrigin := rules[normalizedOrigin]; duplicateOrigin {
			return upstreamRateLimits{}, fmt.Errorf("%w: rule=%d field=origin duplicate=%s", ErrInvalidUpstreamRateLimitConfiguration, configurationIndex, normalizedOrigin)
		}
		rules[normalizedOrigin] = upstreamRateLimitRule{
			maxRequests: configuration.MaxRequests,
			interval:    interval,
		}
	}
	return upstreamRateLimits{rules: rules}, nil
}

func normalizedUpstreamOrigin(rawOrigin string) (string, error) {
	origin := strings.ToLower(strings.TrimSpace(rawOrigin))
	parsedOrigin, parseError := url.Parse(origin)
	if parseError != nil ||
		(parsedOrigin.Scheme != upstreamOriginSchemeHTTPS && parsedOrigin.Scheme != upstreamOriginSchemeHTTP) ||
		parsedOrigin.Host == "" ||
		parsedOrigin.User != nil ||
		parsedOrigin.Path != "" ||
		parsedOrigin.RawPath != "" ||
		parsedOrigin.RawQuery != "" ||
		parsedOrigin.ForceQuery ||
		parsedOrigin.Fragment != "" ||
		parsedOrigin.RawFragment != "" ||
		strings.Contains(origin, "?") ||
		strings.Contains(origin, "#") {
		return "", ErrInvalidUpstreamRateLimitConfiguration
	}
	return parsedOrigin.Scheme + "://" + parsedOrigin.Host, nil
}

func upstreamRequestOrigin(requestURL *url.URL) string {
	return strings.ToLower(requestURL.Scheme) + "://" + strings.ToLower(requestURL.Host)
}
