package interval

import (
	"errors"
	"fmt"
)

const (
	hourly = "hourly"
	daily  = "daily"
	weekly = "weekly"
)

var (
	ErrInvalidDefragInterval = errors.New("invalid interval")
)

// ValidateForDefrag validates that the given interval is a valid option.
// If the value is valid it's returned prepended with a @ ready for use in `CronScheduler#AddFunc()`.
// If the value given is invalid an error is returned.
func ValidateForDefrag(in string) (string, error) {
	switch in {
	case hourly, daily, weekly:
		return "@" + in, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrInvalidDefragInterval, in)
	}
}
