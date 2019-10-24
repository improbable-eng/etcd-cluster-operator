package test

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/util/jsonpath"
)

// CheckStructFields asserts that the struct fields referenced by the supplied expectation path,
// have a value equal to the expectation value.
func CheckStructFields(expectations map[string]interface{}, actual interface{}) (field.ErrorList, error) {
	var allErrs field.ErrorList
	for path, expectedValue := range expectations {
		fldPath := field.NewPath(path)
		jp := jsonpath.New(path)
		err := jp.Parse("{" + path + "}")
		if err != nil {
			return nil, fmt.Errorf("failure in Parse for path %s: %v", path, err)
		}
		results, err := jp.FindResults(actual)
		if err != nil {
			allErrs = append(
				allErrs,
				field.NotFound(fldPath, fmt.Sprintf("%s", err)),
			)
			continue
		}
		if len(results[0]) == 0 {
			allErrs = append(
				allErrs,
				field.Required(fldPath, fmt.Sprintf("no results for path: %s", path)),
			)
			continue
		}
		actualValue := results[0][0].Interface()
		if diff := cmp.Diff(expectedValue, actualValue); diff != "" {
			allErrs = append(
				allErrs,
				field.Invalid(fldPath, actualValue, fmt.Sprintf("mismatch (-expected +actual):\n%s", diff)),
			)
		}
	}
	return allErrs, nil
}

func AssertStructFields(t *testing.T, expectations map[string]interface{}, actual interface{}) bool {
	fieldErrors, err := CheckStructFields(expectations, actual)
	require.NoError(t, err)
	return assert.NoError(t, fieldErrors.ToAggregate())
}
