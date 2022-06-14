package interval

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateForDefrag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		output string
		err    error
	}{
		{"valid value hourly", "hourly", "@hourly", nil},
		{"valid value daily", "daily", "@daily", nil},
		{"valid value weekly", "weekly", "@weekly", nil},
		{"empty", "", "", ErrInvalidDefragInterval},
		{"invalid value", "*/1 * * * *", "", ErrInvalidDefragInterval},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			out, err := ValidateForDefrag(tc.input)

			errorAs(t, err, tc.err)
			assert.Equal(t, tc.output, out)
		})
	}
}

// errorAs wraps assert.ErrorAs but handles nil target errors.
func errorAs(t *testing.T, err, target error) {
	t.Helper()

	if target == nil {
		assert.Nil(t, err)
	} else {
		assert.ErrorAs(t, err, &target)
	}
}
