package proxy

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
)

const (
	upstreamManagementTenant        = "management"
	upstreamDeploymentAccountPrefix = "deployment:"
)

type upstreamRequestScope struct {
	tenant      string
	account     string
	operationID string
	class       upstreamWorkClass
}

type upstreamCapacityUsage struct {
	active       int
	admitted     int
	bulkActive   int
	bulkAdmitted int
}

func (usage upstreamCapacityUsage) canAdmit(limit UpstreamCapacityLimit, class upstreamWorkClass, reserve int) bool {
	return usage.admitted < limit.Admitted && (class == upstreamInteractive || usage.bulkAdmitted < limit.Admitted-reserve)
}

func (usage upstreamCapacityUsage) canActivate(limit UpstreamCapacityLimit, class upstreamWorkClass, reserve int) bool {
	return usage.active < limit.Active && (class == upstreamInteractive || usage.bulkActive < limit.Active-reserve)
}

func (usage *upstreamCapacityUsage) admit(class upstreamWorkClass) {
	usage.admitted++
	if class != upstreamInteractive {
		usage.bulkAdmitted++
	}
}

func (usage *upstreamCapacityUsage) activate(class upstreamWorkClass) {
	usage.active++
	if class != upstreamInteractive {
		usage.bulkActive++
	}
}

func (usage *upstreamCapacityUsage) release(class upstreamWorkClass, active bool) {
	usage.admitted--
	if active {
		usage.active--
	}
	if class != upstreamInteractive {
		usage.bulkAdmitted--
		if active {
			usage.bulkActive--
		}
	}
}

type upstreamAdmissionState uint8

const (
	upstreamAdmissionQueued upstreamAdmissionState = iota
	upstreamAdmissionActive
	upstreamAdmissionReleased
)

type upstreamAdmission struct {
	ctx    context.Context
	scope  upstreamRequestScope
	origin *upstreamOriginAdmission
	tenant *upstreamTenantAdmission
	state  upstreamAdmissionState
}

type upstreamTenantAdmission struct {
	id    string
	usage upstreamCapacityUsage
	queue []*upstreamAdmission
}

type upstreamOriginAdmission struct {
	name         string
	limit        UpstreamCapacityLimit
	usage        upstreamCapacityUsage
	accounts     map[string]upstreamCapacityUsage
	tenants      map[string]*upstreamTenantAdmission
	tenantOrder  []*upstreamTenantAdmission
	tenantCursor int
	rate         upstreamRateLimitRule
	starts       []time.Time
}

// upstreamAdmissionScheduler has no background goroutine. Existing callers wake
// on a capacity change or the next rate-window boundary and select ready work.
// All state below is protected by mutex, including each admission's state.
type upstreamAdmissionScheduler struct {
	mutex   sync.Mutex
	changed chan struct{}
	limits  upstreamCapacity
	usage   upstreamCapacityUsage
	classes [upstreamWorkClassCount]upstreamCapacityUsage
	origins map[string]*upstreamOriginAdmission
	cursor  int
}

func newUpstreamAdmissionScheduler(capacity upstreamCapacity, rates upstreamRateLimits) *upstreamAdmissionScheduler {
	scheduler := &upstreamAdmissionScheduler{
		changed: make(chan struct{}), limits: capacity,
		origins: make(map[string]*upstreamOriginAdmission, len(capacity.origins)),
	}
	for origin, limit := range capacity.origins {
		scheduler.origins[origin] = &upstreamOriginAdmission{
			name: origin, limit: limit, accounts: map[string]upstreamCapacityUsage{},
			tenants: map[string]*upstreamTenantAdmission{}, rate: rates.rules[origin],
		}
	}
	return scheduler
}

// admit is the request boundary. Unknown origins and exhausted allocations never
// enter a queue. A rate-limited request owns only its origin's allocated budget.
func (scheduler *upstreamAdmissionScheduler) admit(ctx context.Context, originName string, scope upstreamRequestScope, now time.Time) (*upstreamAdmission, error) {
	scheduler.mutex.Lock()
	defer scheduler.mutex.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	origin, known := scheduler.origins[originName]
	if !known {
		return nil, backoff.Permanent(fmt.Errorf("%w: origin=%s reason=undeclared", ErrInvalidUpstreamCapacity, originName))
	}
	tenantUsage := upstreamCapacityUsage{}
	tenant := origin.tenants[scope.tenant]
	if tenant != nil {
		tenantUsage = tenant.usage
	}
	accountUsage := origin.accounts[scope.account]
	reserve := scheduler.limits.interactiveReserve
	if !origin.usage.canAdmit(origin.limit, scope.class, reserve) ||
		!tenantUsage.canAdmit(scheduler.limits.tenant, scope.class, reserve) ||
		!accountUsage.canAdmit(scheduler.limits.account, scope.class, reserve) ||
		!scheduler.usage.canAdmit(scheduler.limits.global, scope.class, reserve) ||
		!scheduler.classes[scope.class].canAdmit(scheduler.limits.classes[scope.class], scope.class, 0) {
		return nil, backoff.Permanent(errQueueFull)
	}
	// An origin or principal with all active capacity reserved is interactive-only.
	if scope.class != upstreamInteractive &&
		(origin.limit.Active == reserve || scheduler.limits.tenant.Active == reserve || scheduler.limits.account.Active == reserve) {
		return nil, backoff.Permanent(errQueueFull)
	}
	if tenant == nil {
		tenant = &upstreamTenantAdmission{id: scope.tenant}
		origin.tenants[scope.tenant] = tenant
		origin.tenantOrder = append(origin.tenantOrder, tenant)
	}
	admission := &upstreamAdmission{ctx: ctx, scope: scope, origin: origin, tenant: tenant}
	tenant.queue = append(tenant.queue, admission)
	scheduler.usage.admit(scope.class)
	scheduler.classes[scope.class].admit(scope.class)
	origin.usage.admit(scope.class)
	tenant.usage.admit(scope.class)
	accountUsage.admit(scope.class)
	origin.accounts[scope.account] = accountUsage
	scheduler.signal()
	scheduler.dispatch(now)
	if admission.state == upstreamAdmissionQueued && origin.usage.admitted-origin.usage.active > origin.limit.Admitted-origin.limit.Active {
		scheduler.release(admission)
		return nil, backoff.Permanent(errQueueFull)
	}
	return admission, nil
}

func (scheduler *upstreamAdmissionScheduler) signal() {
	close(scheduler.changed)
	scheduler.changed = make(chan struct{})
}

func (origin *upstreamOriginAdmission) rateWait(now time.Time) time.Duration {
	if origin.rate.maxRequests == 0 {
		return 0
	}
	expired := 0
	for expired < len(origin.starts) && !origin.starts[expired].Add(origin.rate.interval).After(now) {
		expired++
	}
	if expired > 0 {
		origin.starts = slices.Delete(origin.starts, 0, expired)
	}
	if len(origin.starts) == origin.rate.maxRequests {
		return origin.starts[0].Add(origin.rate.interval).Sub(now)
	}
	return 0
}

func (scheduler *upstreamAdmissionScheduler) canActivate(admission *upstreamAdmission) bool {
	class := admission.scope.class
	reserve := scheduler.limits.interactiveReserve
	return scheduler.usage.canActivate(scheduler.limits.global, class, reserve) &&
		scheduler.classes[class].canActivate(scheduler.limits.classes[class], class, 0) &&
		admission.origin.usage.canActivate(admission.origin.limit, class, reserve) &&
		admission.tenant.usage.canActivate(scheduler.limits.tenant, class, reserve) &&
		admission.origin.accounts[admission.scope.account].canActivate(scheduler.limits.account, class, reserve)
}

func (scheduler *upstreamAdmissionScheduler) candidate(origin *upstreamOriginAdmission) (*upstreamAdmission, int, int) {
	for offset := 0; offset < len(origin.tenantOrder); offset++ {
		tenantIndex := (origin.tenantCursor + offset) % len(origin.tenantOrder)
		for requestIndex, admission := range origin.tenantOrder[tenantIndex].queue {
			if admission.ctx.Err() == nil && scheduler.canActivate(admission) {
				return admission, tenantIndex, requestIndex
			}
		}
	}
	return nil, 0, 0
}

// dispatch must run with mutex held. Each selection advances both round-robin
// cursors. A blocked origin or principal cannot consume an active permit.
func (scheduler *upstreamAdmissionScheduler) dispatch(now time.Time) {
	for scheduler.usage.active < scheduler.limits.global.Active {
		selected := false
		for offset := 0; offset < len(scheduler.limits.order); offset++ {
			originIndex := (scheduler.cursor + offset) % len(scheduler.limits.order)
			origin := scheduler.origins[scheduler.limits.order[originIndex]]
			if origin.rateWait(now) > 0 {
				continue
			}
			admission, tenantIndex, requestIndex := scheduler.candidate(origin)
			if admission == nil {
				continue
			}
			admission.tenant.queue = slices.Delete(admission.tenant.queue, requestIndex, requestIndex+1)
			admission.state = upstreamAdmissionActive
			class := admission.scope.class
			scheduler.usage.activate(class)
			scheduler.classes[class].activate(class)
			origin.usage.activate(class)
			admission.tenant.usage.activate(class)
			account := origin.accounts[admission.scope.account]
			account.activate(class)
			origin.accounts[admission.scope.account] = account
			if origin.rate.maxRequests > 0 {
				origin.starts = append(origin.starts, now)
			}
			origin.tenantCursor = tenantIndex + 1
			scheduler.cursor = (originIndex + 1) % len(scheduler.limits.order)
			scheduler.signal()
			selected = true
			break
		}
		if !selected {
			return
		}
	}
}

// nextRateWait returns the earliest rate boundary for otherwise ready work.
// A zero result means that capacity changes or caller cancellation must wake it.
func (scheduler *upstreamAdmissionScheduler) nextRateWait(now time.Time) time.Duration {
	var next time.Duration
	for _, origin := range scheduler.origins {
		wait := origin.rateWait(now)
		if wait <= 0 {
			continue
		}
		if candidate, _, _ := scheduler.candidate(origin); candidate != nil && (next == 0 || wait < next) {
			next = wait
		}
	}
	return next
}

// release must run with mutex held and consumes one queued or active admission.
func (scheduler *upstreamAdmissionScheduler) release(admission *upstreamAdmission) {
	active := admission.state == upstreamAdmissionActive
	if !active {
		index := slices.Index(admission.tenant.queue, admission)
		admission.tenant.queue = slices.Delete(admission.tenant.queue, index, index+1)
	}
	class := admission.scope.class
	scheduler.usage.release(class, active)
	scheduler.classes[class].release(class, active)
	admission.origin.usage.release(class, active)
	admission.tenant.usage.release(class, active)
	account := admission.origin.accounts[admission.scope.account]
	account.release(class, active)
	if account.admitted == 0 {
		delete(admission.origin.accounts, admission.scope.account)
	} else {
		admission.origin.accounts[admission.scope.account] = account
	}
	if admission.tenant.usage.admitted == 0 {
		origin := admission.origin
		index := slices.Index(origin.tenantOrder, admission.tenant)
		origin.tenantOrder = slices.Delete(origin.tenantOrder, index, index+1)
		delete(origin.tenants, admission.scope.tenant)
		if index < origin.tenantCursor {
			origin.tenantCursor--
		}
		if len(origin.tenantOrder) == 0 {
			origin.tenantCursor = 0
		}
	}
	admission.state = upstreamAdmissionReleased
	scheduler.signal()
}
