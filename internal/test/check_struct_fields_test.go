package test

import (
	"testing"

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
		{
			name:          "pointer",
			path:          "",
			expectedValue: pointer.StringPtr("foo"),
			actual:        pointer.StringPtr("foo"),
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
			if tc.expectErr {
				require.Errorf(t, err, "missing error: %#v", tc)
			} else {
				require.NoErrorf(t, err, "unexpected error: %#v", tc)
			}
			t.Log(validationErrors)
		})
	}
}
