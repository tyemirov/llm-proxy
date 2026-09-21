package proxy

import (
	"errors"
	"fmt"
	"sort"
)

// ErrInvalidUpstreamCapacity identifies an invalid upstream capacity contract.
var ErrInvalidUpstreamCapacity = errors.New("invalid_upstream_capacity")

// UpstreamCapacityLimit bounds active requests and all admitted requests.
type UpstreamCapacityLimit struct {
	Active   int `mapstructure:"active" yaml:"active"`
	Admitted int `mapstructure:"admitted" yaml:"admitted"`
}

// UpstreamOriginCapacity assigns one exact origin its active and queued capacity.
type UpstreamOriginCapacity struct {
	Origin   string `mapstructure:"origin" yaml:"origin"`
	Provider string `mapstructure:"provider" yaml:"provider"`
	Active   int    `mapstructure:"active" yaml:"active"`
	Queued   int    `mapstructure:"queued" yaml:"queued"`
}

// UpstreamCapacityConfiguration is the explicit network admission contract.
// Tenant and account limits apply within each origin. Media, status, and transfer
// limits apply across all origins. Accepted durable jobs have separate limits.
type UpstreamCapacityConfiguration struct {
	Global             UpstreamCapacityLimit    `mapstructure:"global" yaml:"global"`
	Tenant             UpstreamCapacityLimit    `mapstructure:"tenant" yaml:"tenant"`
	Account            UpstreamCapacityLimit    `mapstructure:"account" yaml:"account"`
	Media              UpstreamCapacityLimit    `mapstructure:"media" yaml:"media"`
	Status             UpstreamCapacityLimit    `mapstructure:"status" yaml:"status"`
	Transfer           UpstreamCapacityLimit    `mapstructure:"transfer" yaml:"transfer"`
	InteractiveReserve int                      `mapstructure:"interactive_reserve" yaml:"interactive_reserve"`
	Origins            []UpstreamOriginCapacity `mapstructure:"origins" yaml:"origins"`
}

type upstreamWorkClass uint8

const (
	upstreamInteractive upstreamWorkClass = iota
	upstreamMedia
	upstreamStatus
	upstreamTransfer
	upstreamWorkClassCount
)

type upstreamCapacity struct {
	global             UpstreamCapacityLimit
	tenant             UpstreamCapacityLimit
	account            UpstreamCapacityLimit
	classes            [upstreamWorkClassCount]UpstreamCapacityLimit
	interactiveReserve int
	origins            map[string]UpstreamCapacityLimit
	order              []string
}

func newUpstreamCapacity(raw UpstreamCapacityConfiguration, rateLimits upstreamRateLimits) (upstreamCapacity, error) {
	limits := []struct {
		name  string
		value UpstreamCapacityLimit
	}{
		{"global", raw.Global}, {"tenant", raw.Tenant}, {"account", raw.Account},
		{"media", raw.Media}, {"status", raw.Status}, {"transfer", raw.Transfer},
	}
	for _, limit := range limits {
		if limit.value.Active <= 0 || limit.value.Admitted < limit.value.Active {
			return upstreamCapacity{}, fmt.Errorf("%w: field=%s reason=positive_ordered_limits_required", ErrInvalidUpstreamCapacity, limit.name)
		}
		if limit.value.Active > raw.Global.Active || limit.value.Admitted > raw.Global.Admitted {
			return upstreamCapacity{}, fmt.Errorf("%w: field=%s reason=exceeds_global", ErrInvalidUpstreamCapacity, limit.name)
		}
	}
	if raw.InteractiveReserve <= 0 || raw.InteractiveReserve >= raw.Global.Active || raw.InteractiveReserve > raw.Tenant.Active || raw.InteractiveReserve > raw.Account.Active {
		return upstreamCapacity{}, fmt.Errorf("%w: field=interactive_reserve", ErrInvalidUpstreamCapacity)
	}
	capacity := upstreamCapacity{
		global: raw.Global, tenant: raw.Tenant, account: raw.Account,
		classes:            [upstreamWorkClassCount]UpstreamCapacityLimit{raw.Global, raw.Media, raw.Status, raw.Transfer},
		interactiveReserve: raw.InteractiveReserve,
		origins:            make(map[string]UpstreamCapacityLimit, len(raw.Origins)),
	}
	if len(raw.Origins) == 0 {
		return upstreamCapacity{}, fmt.Errorf("%w: field=origins reason=required", ErrInvalidUpstreamCapacity)
	}
	allocated := 0
	for index, rule := range raw.Origins {
		origin, err := normalizedUpstreamOrigin(rule.Origin)
		if err != nil {
			return upstreamCapacity{}, fmt.Errorf("%w: rule=%d field=origin", ErrInvalidUpstreamCapacity, index)
		}
		if _, duplicate := capacity.origins[origin]; duplicate {
			return upstreamCapacity{}, fmt.Errorf("%w: rule=%d origin=%s reason=duplicate", ErrInvalidUpstreamCapacity, index, origin)
		}
		// Subtraction keeps the boundary check safe for overflowing input sums.
		if rule.Active < raw.InteractiveReserve || rule.Active > raw.Global.Active || rule.Queued <= 0 || rule.Queued > raw.Global.Admitted-rule.Active {
			return upstreamCapacity{}, fmt.Errorf("%w: rule=%d origin=%s reason=invalid_limits", ErrInvalidUpstreamCapacity, index, origin)
		}
		admitted := rule.Active + rule.Queued
		if admitted > raw.Global.Admitted-allocated {
			return upstreamCapacity{}, fmt.Errorf("%w: field=global.admitted reason=origin_allocations_exceed_global", ErrInvalidUpstreamCapacity)
		}
		allocated += admitted
		capacity.origins[origin] = UpstreamCapacityLimit{Active: rule.Active, Admitted: admitted}
		capacity.order = append(capacity.order, origin)
	}
	for origin := range rateLimits.rules {
		if _, declared := capacity.origins[origin]; !declared {
			return upstreamCapacity{}, fmt.Errorf("%w: origin=%s reason=rate_limit_origin_missing", ErrInvalidUpstreamCapacity, origin)
		}
	}
	sort.Strings(capacity.order)
	return capacity, nil
}
