package delivery

import "time"

type RetryPolicy struct {
	MaxAttempts int
	Delay       time.Duration
}

type RetryDecision struct {
	Status        string
	Attempt       int
	NextAttemptAt time.Time
	Inactive      bool
}

func DecideRetry(status string, previousAttempt int, attemptedAt time.Time, policy RetryPolicy) RetryDecision {
	nextAttempt := previousAttempt + 1
	if status == "blocked" {
		return RetryDecision{Status: "blocked", Attempt: nextAttempt, Inactive: true}
	}
	if status == "success" {
		return RetryDecision{Status: "sent", Attempt: nextAttempt}
	}
	maxAttempts := policy.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	if nextAttempt >= maxAttempts {
		return RetryDecision{Status: "failed", Attempt: nextAttempt}
	}
	delay := policy.Delay
	if delay <= 0 {
		delay = 15 * time.Minute
	}
	return RetryDecision{
		Status:        "retry",
		Attempt:       nextAttempt,
		NextAttemptAt: attemptedAt.Add(delay),
	}
}
