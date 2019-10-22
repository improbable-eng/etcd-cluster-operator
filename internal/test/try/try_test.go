package try

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
			tick:      time.Millisecond * 5,
			duration:  time.Millisecond * 50,
			expectErr: false,
		},
		{
			name: "TestEventually_AlwaysErroring_Fails",
			fn: func(_ int) error {
				return errors.New("foo")
			},
			tick:      time.Millisecond * 5,
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
			tick:      time.Millisecond * 5,
			duration:  time.Millisecond * 50,
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

// TODO(wallrj): Add tests for failure cases.
// For this I searched for a way to modify / wrap `testing.T` so that it fails
// if the there are no assertion errors, and succeeds if there are errors, with
// some way to check the assertion message.
func TestCheckStructFields(t *testing.T) {
	for _, tc := range []struct {
		name          string
		path          string
		expectedValue interface{}
		actual        interface{}
	}{
		{
			name:          "int values",
			path:          "",
			expectedValue: 123,
			actual:        123,
		},
		{
			name:          "string values",
			path:          "",
			expectedValue: "foo",
			actual:        "foo",
		},
		{
			name:          "quantity values",
			path:          "",
			expectedValue: resource.MustParse("123Gi"),
			actual:        resource.MustParse("123Gi"),
		},
		// TODO: This currently panics if you supply two different pointers.
		// See https://github.com/stretchr/testify/pull/680
		// And https://github.com/stretchr/testify/issues/677
		// {
		//	name:          "pointer",
		//	path:          "",
		//	expectedValue: pointer.StringPtr("foo"),
		//	actual:        pointer.StringPtr("foox"),
		// },
		{
			name:          "struct paths with maps",
			path:          ".spec.resources.requests.storage",
			expectedValue: resource.MustParse("101Gi"),
			actual: &corev1.PersistentVolumeClaim{
				Spec: corev1.PersistentVolumeClaimSpec{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"storage": resource.MustParse("101Gi"),
						},
					},
				},
			},
		},
		{
			name:          "struct paths with lists",
			path:          `.containers[?(@.name=="container2")].image`,
			expectedValue: "example.com/image2:v1",
			actual: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "container1",
						Image: "example.com/image1:v1",
					},
					{
						Name:  "container2",
						Image: "example.com/image2:v1",
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			CheckStructFields(
				t,
				map[string]interface{}{
					tc.path: tc.expectedValue,
				},
				tc.actual,
			)
		})
	}
}
