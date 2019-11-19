package try

import (
	"errors"
	"time"
)

// Consistently checks that `fn' consistently returns no error each `tick' for `duration'. It returns an error if any
// call to `fn' errors in the given time window.
func Consistently(fn func() error, duration time.Duration, tick time.Duration) error {
	timeout := time.After(duration)
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-timeout:
			return nil
		case <-ticker.C:
			if err := fn(); err != nil {
				return err
			}
		}
	}
}

// Eventually checks that `fn' eventually stops erroring, by calling `fn' every `tick' until it times out after
// `duration'. It returns nil if `fn' stops erroring, otherwise the last error from `fn' is returned.
func Eventually(fn func() error, duration time.Duration, tick time.Duration) error {
	timeout := time.After(duration)
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	var lastErr error
	for {
		select {
		case <-timeout:
			if lastErr == nil {
				return errors.New("function failed to return at least once")
			}
			return lastErr
		case <-ticker.C:
			lastErr = fn()
			if lastErr == nil {
				return nil
			}
		}
	}
}
