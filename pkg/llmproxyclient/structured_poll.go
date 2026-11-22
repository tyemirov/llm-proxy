package llmproxyclient

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const headerRetryAfter = "Retry-After"

// retryAfterAdvice retains parsed timing only, without the untrusted header text.
type retryAfterAdvice struct {
	delay    time.Duration
	deadline time.Time
}

func parseRetryAfter(value string) retryAfterAdvice {
	value = strings.TrimSpace(value)
	if value == "" {
		return retryAfterAdvice{}
	}
	decimal := true
	for _, char := range value {
		if char < '0' || char > '9' {
			decimal = false
			break
		}
	}
	if decimal {
		seconds, err := strconv.ParseUint(value, 10, 64)
		if err != nil || seconds > math.MaxInt64/uint64(time.Second) {
			return retryAfterAdvice{delay: time.Duration(math.MaxInt64)}
		}
		return retryAfterAdvice{delay: time.Duration(seconds) * time.Second}
	}
	deadline, err := http.ParseTime(value)
	if err != nil {
		return retryAfterAdvice{}
	}
	return retryAfterAdvice{deadline: deadline}
}

func (advice retryAfterAdvice) at(now time.Time) time.Time {
	if advice.delay > 0 {
		return now.Add(advice.delay)
	}
	if advice.deadline.After(now) {
		return advice.deadline
	}
	return time.Time{}
}

// RetryAfter returns the response's future Retry-After deadline, or zero when
// the header is absent, malformed, zero, or an HTTP date at or before now.
// Delta seconds are relative to now and saturate at the largest time.Duration.
func (failure *HTTPFailure) RetryAfter(now time.Time) time.Time {
	return failure.retryAfter.at(now)
}

// RetryAfter returns the snapshot's future Retry-After deadline, or zero when
// the header is absent, malformed, zero, or an HTTP date at or before now.
// Delta seconds are relative to now and saturate at the largest time.Duration.
func (result StructuredRequestResult) RetryAfter(now time.Time) time.Time {
	return result.retryAfter.at(now)
}

type responseRetryError struct {
	error
	advice retryAfterAdvice
}

func (failure *responseRetryError) Unwrap() error { return failure.error }

func (failure *responseRetryError) RetryAfter(now time.Time) time.Time {
	return failure.advice.at(now)
}

func withResponseRetryAfter(err error, advice retryAfterAdvice) error {
	if advice == (retryAfterAdvice{}) {
		return err
	}
	return &responseRetryError{error: err, advice: advice}
}

// ResponseRetryAfter returns the future deadline carried by a response error.
// It returns zero when the wrapped error carries no future deadline.
func ResponseRetryAfter(now time.Time, err error) time.Time {
	var advice interface{ RetryAfter(time.Time) time.Time }
	if errors.As(err, &advice) {
		return advice.RetryAfter(now)
	}
	return time.Time{}
}

// StructuredRequestPollAt advises when the caller can next read the original
// request. Attempts are 1-based (zero also means first), with delays of 10, 20,
// 40, then 60 seconds. Valid Retry-After metadata raises that minimum, including
// above the cap. Response errors also contribute their retained retry metadata.
// The caller owns scheduling and settlement.
func StructuredRequestPollAt(now time.Time, attempt uint64, result StructuredRequestResult, err error) time.Time {
	delay := time.Minute
	switch attempt {
	case 0, 1:
		delay = 10 * time.Second
	case 2:
		delay = 20 * time.Second
	case 3:
		delay = 40 * time.Second
	}
	deadline := now.Add(delay)
	if minimum := result.RetryAfter(now); minimum.After(deadline) {
		deadline = minimum
	}
	if minimum := ResponseRetryAfter(now, err); minimum.After(deadline) {
		deadline = minimum
	}
	return deadline
}
