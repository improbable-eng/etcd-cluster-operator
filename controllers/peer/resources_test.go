package peer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/pointer"
)

func TestGoMaxProcs(t *testing.T) {
	tests := map[string]struct {
		limit    string
		expected *int64
	}{
		"Negative": {
			limit:    "-1",
			expected: nil,
		},
		"Zero": {
			limit:    "0",
			expected: nil,
		},
		"JustAboveZero": {
			limit:    "0.1",
			expected: pointer.Int64Ptr(1),
		},
		"PointFive": {
			limit:    "0.5",
			expected: pointer.Int64Ptr(1),
		},
		"AlmostOne": {
			limit:    "0.9",
			expected: pointer.Int64Ptr(1),
		},
		"ExactlyOne": {
			limit:    "1",
			expected: pointer.Int64Ptr(1),
		},
		"JustAboveOne": {
			limit:    "1.1",
			expected: pointer.Int64Ptr(1),
		},
		"OnePointFive": {
			limit:    "1.5",
			expected: pointer.Int64Ptr(1),
		},
		"AlmostTwo": {
			limit:    "1.9",
			expected: pointer.Int64Ptr(1),
		},
		"ExactlyTwo": {
			limit:    "2",
			expected: pointer.Int64Ptr(2),
		},
		"TwoPointFive": {
			limit:    "2.5",
			expected: pointer.Int64Ptr(2),
		},
		"AlmostThree": {
			limit:    "2.9",
			expected: pointer.Int64Ptr(2),
		},
	}

	for title, tc := range tests {
		t.Run(title, func(t *testing.T) {
			actual := goMaxProcs(resource.MustParse(tc.limit))
			if tc.expected == nil {
				assert.Nil(t, actual)
			} else {
				assert.Equal(t, *tc.expected, *actual)
			}
		})
	}

}
