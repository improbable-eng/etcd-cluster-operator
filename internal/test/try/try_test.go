package try

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConsistently(t *testing.T) {
	for _, tc := range []struct {
		name      string
		fn        func(int) error
		tick      time.Duration
		duration  time.Duration
		expectErr bool
	}{
		{
			name: "TestConsistently_NeverErroring_DoesNotReturnError",
			fn: func(_ int) error {
				return nil
			},
			tick:      time.Millisecond * 5,
			duration:  time.Millisecond * 50,
			expectErr: false,
		},
		{
			name: "TestConsistently_AlwaysErroring_ReturnsError",
			fn: func(_ int) error {
				return errors.New("foo")
			},
			tick:      time.Millisecond * 5,
			duration:  time.Millisecond * 50,
			expectErr: true,
		},
		{
			name: "TestConsistently_SometimesErroring_ReturnsError",
			fn: func(state int) error {
				if state%2 == 0 {
					return nil
				} else {
					return errors.New("foo")
				}
			},
			tick:      time.Millisecond * 5,
			duration:  time.Millisecond * 100,
			expectErr: true,
		},
		{
			name: "TestConsistently_ErrorAfterDuration_MissesError",
			fn: func(state int) error {
				if state == 11 {
					return errors.New("foo")
				} else {
					return nil
				}
			},
			tick:      time.Millisecond * 5,
			duration:  time.Millisecond * 50,
			expectErr: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var i int
			err := Consistently(func() error {
				err := tc.fn(i)
				i++
				return err
			}, tc.duration, tc.tick)
			if tc.expectErr {
				require.Error(t, err, "an error was not found, but one was expected")
			} else {
				require.NoError(t, err, "an error was found, but not expected")
			}
		})
	}
}

func TestEventually(t *testing.T) {
	for _, tc := range []struct {
		name      string
		fn        func(int) error
		tick      time.Duration
		duration  time.Duration
		expectErr bool
	}{
		{
			name: "TestEventually_NeverErroring_Succeeds",
			fn: func(_ int) error {
				return nil
			},
			tick:      time.Millisecond * 1,
			duration:  time.Millisecond * 50,
			expectErr: false,
		},
		{
			name: "TestEventually_AlwaysErroring_Fails",
			fn: func(_ int) error {
				return errors.New("foo")
			},
			tick:      time.Millisecond * 1,
			duration:  time.Millisecond * 50,
			expectErr: true,
		},
		{
			name: "TestEventually_InitiallyErroring_EventuallySucceeds",
			fn: func(state int) error {
				if state == 3 {
					return nil
				}
				return errors.New("foo")
			},
			tick:      time.Millisecond * 1,
			duration:  time.Millisecond * 100,
			expectErr: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var i int
			err := Eventually(func() error {
				err := tc.fn(i)
				i++
				return err
			}, tc.duration, tc.tick)
			if tc.expectErr {
				require.Error(t, err, "an error was not found, but one was expected")
				require.Equal(t, "foo", err.Error())
			} else {
				require.NoError(t, err, "an error was found, but not expected")
			}
		})
	}
}
