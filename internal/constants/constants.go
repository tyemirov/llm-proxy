package constants

const (
	// LogFieldError identifies the structured log field name for an error.
	LogFieldError = "error"

	// LogFieldLatencyMilliseconds identifies the structured log field name for latency in milliseconds.
	LogFieldLatencyMilliseconds = "latency_ms"

	// LogEventReadResponseBodyFailed identifies failures while reading an HTTP response body.
	LogEventReadResponseBodyFailed = "read response body failed"
	// LogEventUpstreamRateLimitDelayed identifies an upstream call delayed by the shared rate limiter.
	LogEventUpstreamRateLimitDelayed = "upstream HTTP call delayed by rate limit"
	// LogEventUpstreamRateLimitCanceled identifies an upstream call canceled while waiting for the shared rate limiter.
	LogEventUpstreamRateLimitCanceled = "upstream HTTP call canceled during rate limit wait"
	// LogFieldUpstreamOrigin identifies the normalized upstream origin affected by rate limiting.
	LogFieldUpstreamOrigin = "upstream_origin"
	// LogFieldRateLimitMaxRequests identifies the configured request ceiling for one interval.
	LogFieldRateLimitMaxRequests = "rate_limit_max_requests"
	// LogFieldRateLimitInterval identifies the configured rolling-window interval.
	LogFieldRateLimitInterval = "rate_limit_interval"
	// LogFieldRateLimitInitialWaitMilliseconds identifies the first wait imposed on a call.
	LogFieldRateLimitInitialWaitMilliseconds = "rate_limit_initial_wait_ms"
	// LogFieldRateLimitTotalWaitMilliseconds identifies the total wait imposed on a call.
	LogFieldRateLimitTotalWaitMilliseconds = "rate_limit_total_wait_ms"

	EmptyString = ""
	LineBreak   = "\n"
)
