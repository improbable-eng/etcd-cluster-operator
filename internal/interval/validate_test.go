package interval

import (
	"errors"
	"testing"
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

			if tc.err != nil && err == nil {
				t.Errorf("expected an error")
			}
			if err != nil && tc.err == nil {
				t.Errorf("unexpected error: %s", err)
			}
			if tc.err != nil && err != nil {
				if !errors.As(err, &tc.err) {
					t.Errorf("expected (%s) got (%s)", tc.err, err)
				}
			}

			if out != tc.output {
				t.Errorf("expected (%s) got (%s)", tc.output, out)
			}
		})
	}
}
