package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/pointer"
)

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
		expectErr     bool
	}{
		{
			name:          "int match",
			path:          "",
			expectedValue: 123,
			actual:        123,
		},
		{
			name:          "int mismatch",
			path:          "",
			expectedValue: 123,
			actual:        124,
			expectErr:     true,
		},
		{
			name:          "string match",
			path:          "",
			expectedValue: "foo",
			actual:        "foo",
		},
		{
			name:          "string mismatch",
			path:          "",
			expectedValue: "foo",
			actual:        "fOo",
			expectErr:     true,
		},
		{
			name:          "quantity match",
			path:          "",
			expectedValue: resource.MustParse("123Gi"),
			actual:        resource.MustParse("123Gi"),
		},
		{
			name:          "quantity mismatch",
			path:          "",
			expectedValue: resource.MustParse("123Gi"),
			actual:        resource.MustParse("123Mi"),
			expectErr:     true,
		},
		{
			name:          "pointer match",
			path:          "",
			expectedValue: pointer.StringPtr("foo"),
			actual:        pointer.StringPtr("foo"),
		},
		{
			name:          "pointer mismatch",
			path:          "",
			expectedValue: pointer.StringPtr("foo"),
			actual:        pointer.StringPtr("fOo"),
			expectErr:     true,
		},
		{
			name:          "simple path match",
			path:          ".Bar",
			expectedValue: "BAR",
			actual: struct {
				Foo string
				Bar string
			}{
				Foo: "FOO",
				Bar: "BAR",
			},
		},
		{
			name:          "simple path missing",
			path:          ".Baz",
			expectedValue: nil,
			actual: struct {
				Foo string
				Bar string
			}{
				Foo: "FOO",
				Bar: "BAR",
			},
			expectErr: true,
		},
		{
			name:          "simple map key match",
			path:          ".bar",
			expectedValue: "BAR",
			actual: map[string]string{
				"foo": "FOO",
				"bar": "BAR",
			},
		},
		{
			name:          "simple map key missing",
			path:          ".baz",
			expectedValue: "BAZ",
			actual: map[string]string{
				"foo": "FOO",
				"bar": "BAR",
			},
			expectErr: true,
		},
		{
			name:          "simple list index match",
			path:          "[1]",
			expectedValue: "bar",
			actual:        []string{"foo", "bar"},
		},
		{
			name:          "simple list index missing",
			path:          "[2]",
			expectedValue: nil,
			actual:        []string{"foo", "bar"},
			expectErr:     true,
		},
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
			validationErrors, err := CheckStructFields(
				map[string]interface{}{
					tc.path: tc.expectedValue,
				},
				tc.actual,
			)
			require.NoError(t, err)
			if tc.expectErr {
				assert.Errorf(t, validationErrors.ToAggregate(), "missing errors")
				t.Log(validationErrors)
			} else {
				assert.NoErrorf(t, validationErrors.ToAggregate(), "unexpected errors")
			}
		})
	}
}