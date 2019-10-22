package try

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/util/jsonpath"
)

// Consistently checks that `fn' consistently returns no error each `tick' for `duration'. It returns an error if any
// call to `fn' errors in the given time window.
func Consistently(fn func() error, duration time.Duration, tick time.Duration) error {
	timeout := time.After(duration)
	ticker := time.Tick(tick)
	for {
		select {
		case <-timeout:
			return nil
		case <-ticker:
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
	ticker := time.Tick(tick)
	var lastErr error
	for {
		select {
		case <-timeout:
			if lastErr == nil {
				return errors.New("function failed to return at least once")
			}
			return lastErr
		case <-ticker:
			lastErr = fn()
			if lastErr == nil {
				return nil
			}
		}
	}
}

// CheckStructFields asserts that the struct fields referenced by the supplied expectation path,
// have a value equal to the expectation value.
// TODO: This currently panics if you supply two different pointers.
// See https://github.com/stretchr/testify/pull/680
// And https://github.com/stretchr/testify/issues/677
func CheckStructFields(t *testing.T, expectations map[string]interface{}, actual interface{}) {
	for path, expectedValue := range expectations {
		jp := jsonpath.New(path)
		err := jp.Parse("{" + path + "}")
		if !assert.NoErrorf(t, err, "jsonpath: %v", path) {
			continue
		}
		results, err := jp.FindResults(actual)
		if !assert.NoErrorf(t, err, "jsonpath: %v", path) {
			continue
		}
		if len(results[0]) == 0 {
			assert.Failf(t, "field not found", "jsonpath: %v", path)
			continue
		}
		assert.Equalf(t, expectedValue, results[0][0].Interface(), "jsonpath: %v", path)
	}
}
