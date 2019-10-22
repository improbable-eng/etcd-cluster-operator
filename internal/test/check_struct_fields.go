package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/util/jsonpath"
)

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
