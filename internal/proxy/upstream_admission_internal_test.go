package proxy

import (
	"context"
	"errors"
	"testing"
	"time"
)

const (
	admissionOriginA = "https://a.example"
	admissionOriginB = "https://b.example"
)

func admissionTestConfiguration() UpstreamCapacityConfiguration {
	return UpstreamCapacityConfiguration{
		Global:             UpstreamCapacityLimit{Active: 4, Admitted: 16},
		Tenant:             UpstreamCapacityLimit{Active: 3, Admitted: 8},
		Account:            UpstreamCapacityLimit{Active: 3, Admitted: 8},
		Media:              UpstreamCapacityLimit{Active: 3, Admitted: 8},
		Status:             UpstreamCapacityLimit{Active: 3, Admitted: 8},
		Transfer:           UpstreamCapacityLimit{Active: 3, Admitted: 8},
		InteractiveReserve: 1,
		Origins: []UpstreamOriginCapacity{
			{Origin: admissionOriginA, Active: 3, Queued: 5},
			{Origin: admissionOriginB, Active: 3, Queued: 5},
		},
	}
}

func admissionTestScheduler(t *testing.T, raw UpstreamCapacityConfiguration, rates upstreamRateLimits) *upstreamAdmissionScheduler {
	t.Helper()
	capacity, err := newUpstreamCapacity(raw, rates)
	if err != nil {
		t.Fatal(err)
	}
	return newUpstreamAdmissionScheduler(capacity, rates)
}

func admitTestRequest(t *testing.T, scheduler *upstreamAdmissionScheduler, origin, tenant, account string, class upstreamWorkClass, now time.Time) *upstreamAdmission {
	t.Helper()
	admission, err := scheduler.admit(context.Background(), origin, upstreamRequestScope{tenant: tenant, account: account, class: class}, now)
	if err != nil {
		t.Fatal(err)
	}
	return admission
}

func releaseTestAdmission(scheduler *upstreamAdmissionScheduler, admission *upstreamAdmission, now time.Time) {
	scheduler.mutex.Lock()
	defer scheduler.mutex.Unlock()
	scheduler.release(admission)
	scheduler.dispatch(now)
}

func TestUpstreamAdmissionSchedulerReservesInteractiveProgressAcrossMediaClasses(t *testing.T) {
	for _, class := range []upstreamWorkClass{upstreamMedia, upstreamStatus, upstreamTransfer} {
		t.Run(string(rune('0'+class)), func(t *testing.T) {
			scheduler := admissionTestScheduler(t, admissionTestConfiguration(), upstreamRateLimits{})
			now := time.Unix(100, 0)
			first := admitTestRequest(t, scheduler, admissionOriginA, "tenant", "shared-account", class, now)
			second := admitTestRequest(t, scheduler, admissionOriginA, "tenant", "shared-account", class, now)
			waiting := admitTestRequest(t, scheduler, admissionOriginA, "tenant", "shared-account", class, now)
			interactive := admitTestRequest(t, scheduler, admissionOriginA, "tenant", "shared-account", upstreamInteractive, now)
			otherOrigin := admitTestRequest(t, scheduler, admissionOriginB, "tenant", "shared-account", upstreamInteractive, now)
			if first.state != upstreamAdmissionActive || second.state != upstreamAdmissionActive || waiting.state != upstreamAdmissionQueued || interactive.state != upstreamAdmissionActive || otherOrigin.state != upstreamAdmissionActive {
				t.Fatal("media saturation did not preserve same-origin and independent-origin interactive progress")
			}
			if scheduler.usage.active != 4 || scheduler.usage.admitted != 5 {
				t.Fatalf("global usage=%+v", scheduler.usage)
			}
			for _, admission := range []*upstreamAdmission{first, second, waiting, interactive, otherOrigin} {
				releaseTestAdmission(scheduler, admission, now)
			}
			if scheduler.usage != (upstreamCapacityUsage{}) || len(scheduler.origins[admissionOriginA].tenants) != 0 || len(scheduler.origins[admissionOriginA].accounts) != 0 {
				t.Fatal("terminal work retained capacity or principal state")
			}
		})
	}
}

func TestUpstreamAdmissionSchedulerSharesAccountAndRotatesTenants(t *testing.T) {
	raw := admissionTestConfiguration()
	raw.Account.Active = 1
	scheduler := admissionTestScheduler(t, raw, upstreamRateLimits{})
	now := time.Unix(100, 0)
	first := admitTestRequest(t, scheduler, admissionOriginA, "tenant-a", "account", upstreamInteractive, now)
	secondA := admitTestRequest(t, scheduler, admissionOriginA, "tenant-a", "account", upstreamInteractive, now)
	secondB := admitTestRequest(t, scheduler, admissionOriginA, "tenant-b", "account", upstreamInteractive, now)
	if secondA.state != upstreamAdmissionQueued || secondB.state != upstreamAdmissionQueued {
		t.Fatal("the same provider account exceeded its active limit across tenants")
	}
	releaseTestAdmission(scheduler, first, now)
	if secondB.state != upstreamAdmissionActive || secondA.state != upstreamAdmissionQueued {
		t.Fatal("queued tenant did not get its turn after the active tenant released capacity")
	}
	releaseTestAdmission(scheduler, secondB, now)
	if secondA.state != upstreamAdmissionActive {
		t.Fatal("the original tenant did not progress on the next turn")
	}
	releaseTestAdmission(scheduler, secondA, now)
}

func TestUpstreamAdmissionSchedulerRotatesOriginsAtGlobalCeiling(t *testing.T) {
	raw := admissionTestConfiguration()
	raw.Global.Active = 3
	scheduler := admissionTestScheduler(t, raw, upstreamRateLimits{})
	now := time.Unix(100, 0)
	active := []*upstreamAdmission{}
	for index := 0; index < 3; index++ {
		active = append(active, admitTestRequest(t, scheduler, admissionOriginA, "tenant", "account", upstreamInteractive, now))
	}
	waitingA := admitTestRequest(t, scheduler, admissionOriginA, "tenant", "account", upstreamInteractive, now)
	waitingB := admitTestRequest(t, scheduler, admissionOriginB, "tenant", "account", upstreamInteractive, now)
	releaseTestAdmission(scheduler, active[0], now)
	if waitingB.state != upstreamAdmissionActive || waitingA.state != upstreamAdmissionQueued || scheduler.usage.active != 3 {
		t.Fatal("the independent origin did not receive the next available global slot")
	}
	releaseTestAdmission(scheduler, active[1], now)
	if waitingA.state != upstreamAdmissionActive || scheduler.usage.active != 3 {
		t.Fatal("round-robin origin service did not return to the first origin")
	}
	for _, admission := range []*upstreamAdmission{active[2], waitingA, waitingB} {
		releaseTestAdmission(scheduler, admission, now)
	}
}

func TestUpstreamAdmissionSchedulerIsolatesRateWaitAndCancellation(t *testing.T) {
	raw := admissionTestConfiguration()
	raw.Origins[0].Queued = 1
	rates := upstreamRateLimits{rules: map[string]upstreamRateLimitRule{admissionOriginA: {maxRequests: 1, interval: time.Second}}}
	scheduler := admissionTestScheduler(t, raw, rates)
	now := time.Unix(100, 0)
	first := admitTestRequest(t, scheduler, admissionOriginA, "tenant", "account", upstreamInteractive, now)
	releaseTestAdmission(scheduler, first, now)
	ctx, cancel := context.WithCancel(context.Background())
	waiting, err := scheduler.admit(ctx, admissionOriginA, upstreamRequestScope{tenant: "tenant", account: "account"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if scheduler.usage.active != 0 || waiting.state != upstreamAdmissionQueued || scheduler.nextRateWait(now) != time.Second {
		t.Fatal("rate wait consumed active capacity or lost its wake boundary")
	}
	if _, err := scheduler.admit(context.Background(), admissionOriginA, upstreamRequestScope{tenant: "tenant", account: "account"}, now); !errors.Is(err, errQueueFull) {
		t.Fatalf("rate-limited origin queue overflow error=%v", err)
	}
	other := admitTestRequest(t, scheduler, admissionOriginB, "tenant", "account", upstreamInteractive, now)
	if other.state != upstreamAdmissionActive {
		t.Fatal("rate wait blocked the independent origin")
	}
	cancel()
	releaseTestAdmission(scheduler, waiting, now)
	if _, err := scheduler.admit(ctx, admissionOriginA, upstreamRequestScope{tenant: "tenant", account: "account"}, now); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ingress error=%v", err)
	}
	replacement := admitTestRequest(t, scheduler, admissionOriginA, "tenant", "account", upstreamInteractive, now)
	scheduler.mutex.Lock()
	scheduler.dispatch(now.Add(time.Second))
	scheduler.mutex.Unlock()
	if replacement.state != upstreamAdmissionActive {
		t.Fatal("rate-window expiry did not dispatch the replacement")
	}
	releaseTestAdmission(scheduler, replacement, now.Add(time.Second))
	releaseTestAdmission(scheduler, other, now.Add(time.Second))
}

func TestUpstreamCapacityRejectsIncompleteOrContradictoryConfiguration(t *testing.T) {
	cases := []struct {
		name   string
		change func(*UpstreamCapacityConfiguration)
	}{
		{"missing origins", func(c *UpstreamCapacityConfiguration) { c.Origins = nil }},
		{"zero global active", func(c *UpstreamCapacityConfiguration) { c.Global.Active = 0 }},
		{"admitted below active", func(c *UpstreamCapacityConfiguration) { c.Media.Admitted = 1 }},
		{"tenant above global", func(c *UpstreamCapacityConfiguration) { c.Tenant.Active = 5 }},
		{"zero reserve", func(c *UpstreamCapacityConfiguration) { c.InteractiveReserve = 0 }},
		{"reserve above tenant", func(c *UpstreamCapacityConfiguration) { c.InteractiveReserve = 4 }},
		{"duplicate origin", func(c *UpstreamCapacityConfiguration) { c.Origins[1].Origin = "HTTPS://A.EXAMPLE" }},
		{"origin path", func(c *UpstreamCapacityConfiguration) { c.Origins[0].Origin += "/path" }},
		{"zero queue", func(c *UpstreamCapacityConfiguration) { c.Origins[0].Queued = 0 }},
		{"origin above global", func(c *UpstreamCapacityConfiguration) { c.Origins[0].Active = 5 }},
		{"shared admission overflow", func(c *UpstreamCapacityConfiguration) { c.Global.Admitted = 15 }},
		{"overflowing queue", func(c *UpstreamCapacityConfiguration) { c.Origins[0].Queued = int(^uint(0) >> 1) }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			raw := admissionTestConfiguration()
			testCase.change(&raw)
			if _, err := newUpstreamCapacity(raw, upstreamRateLimits{}); !errors.Is(err, ErrInvalidUpstreamCapacity) {
				t.Fatalf("invalid capacity error=%v", err)
			}
		})
	}
	rates := upstreamRateLimits{rules: map[string]upstreamRateLimitRule{"https://unknown.example": {maxRequests: 1, interval: time.Second}}}
	if _, err := newUpstreamCapacity(admissionTestConfiguration(), rates); !errors.Is(err, ErrInvalidUpstreamCapacity) {
		t.Fatalf("unknown rate-limit origin error=%v", err)
	}
	scheduler := admissionTestScheduler(t, admissionTestConfiguration(), upstreamRateLimits{})
	if _, err := scheduler.admit(context.Background(), "https://unknown.example", upstreamRequestScope{}, time.Now()); !errors.Is(err, ErrInvalidUpstreamCapacity) {
		t.Fatalf("undeclared request origin error=%v", err)
	}
}
