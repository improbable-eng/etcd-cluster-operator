package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/pointer"
)

func TestCheckStructFields(t *testing.T) {
	for _, tc := range []struct {
		name          string
		path          string
		expectedValue interface{}
		actual        interface{}
		expectErr     bool
	}{
		{
			name:          "IntMatch",
			path:          "",
			expectedValue: 123,
			actual:        123,
		},
		{
			name:          "IntMismatch",
			path:          "",
			expectedValue: 123,
			actual:        124,
			expectErr:     true,
		},
		{
			name:          "StringMatch",
			path:          "",
			expectedValue: "foo",
			actual:        "foo",
		},
		{
			name:          "StringMismatch",
			path:          "",
			expectedValue: "foo",
			actual:        "fOo",
			expectErr:     true,
		},
		{
			name:          "QuantityMatch",
			path:          "",
			expectedValue: resource.MustParse("123Gi"),
			actual:        resource.MustParse("123Gi"),
		},
		{
			name:          "QuantityMismatch",
			path:          "",
			expectedValue: resource.MustParse("123Gi"),
			actual:        resource.MustParse("123Mi"),
			expectErr:     true,
		},
		{
			name:          "PointerMatch",
			path:          "",
			expectedValue: pointer.StringPtr("foo"),
			actual:        pointer.StringPtr("foo"),
		},
		{
			name:          "PointerMismatch",
			path:          "",
			expectedValue: pointer.StringPtr("foo"),
			actual:        pointer.StringPtr("fOo"),
			expectErr:     true,
		},
		{
			name:          "SimplePathMatch",
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
			name:          "SimplePathMissing",
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
			name:          "SimpleMapKeyMatch",
			path:          ".bar",
			expectedValue: "BAR",
			actual: map[string]string{
				"foo": "FOO",
				"bar": "BAR",
			},
		},
		{
			name:          "SimpleMapKeyMissing",
			path:          ".baz",
			expectedValue: "BAZ",
			actual: map[string]string{
				"foo": "FOO",
				"bar": "BAR",
			},
			expectErr: true,
		},
		{
			name:          "SimpleListIndexMatch",
			path:          "[1]",
			expectedValue: "bar",
			actual:        []string{"foo", "bar"},
		},
		{
			name:          "SimpleListIndexMissing",
			path:          "[2]",
			expectedValue: nil,
			actual:        []string{"foo", "bar"},
			expectErr:     true,
		},
		{
			name:          "StructPathsWithMaps",
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
			name:          "StructPathsWithLists",
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
