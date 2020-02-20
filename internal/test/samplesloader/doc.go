// Package samplesloader has convenience functions for loading the Yaml manifests in `config/samples`.
// Each etcd.improbable.io v1alpha1 resource kind has a corresponding function
// which allows it to be loaded with an overridden namespace.
// These are handy for use in tests and has the added benefit of testing that
// the sample manifests are correct.
package samplesloader
