package delivery

import (
	"fmt"
	"time"
)

func IsDailyScheduleDue(now time.Time, lastRun time.Time, clockValue string, timezone string) (bool, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return false, fmt.Errorf("load timezone: %w", err)
	}
	clock, err := time.Parse("15:04", clockValue)
	if err != nil {
		return false, fmt.Errorf("parse schedule time: %w", err)
	}

	localNow := now.In(location)
	dueAt := time.Date(
		localNow.Year(),
		localNow.Month(),
		localNow.Day(),
		clock.Hour(),
		clock.Minute(),
		0,
		0,
		location,
	)
	if localNow.Before(dueAt) {
		return false, nil
	}
	if lastRun.IsZero() {
		return true, nil
	}
	return lastRun.In(location).Before(dueAt), nil
}

func DigestDate(now time.Time, timezone string) (string, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return "", fmt.Errorf("load timezone: %w", err)
	}
	return now.In(location).Format("2006-01-02"), nil
}
