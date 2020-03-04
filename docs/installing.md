# Installation Guide

## Deploying webhooks / Installing Cert-Manager

This Operator uses [defaulting and validating webhooks](https://book.kubebuilder.io/reference/webhook-overview.html)
to validate any `EtcdCluster` and `EtcdPeer` custom resources that you create,
and to assign default values for any optional fields in those APIs.

**You are strongly advised to install these webhooks** in order to prevent unsupported `EtcdCluster` configurations,
and to prevent unsupported configuration changes.
[The webhook APIs used by this Operator were introduced in Kubernetes 1.9](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG-1.9.md#api-machinery).

Running `make deploy`  will add [Webhook configuration](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/#webhook-configuration) such that,
for any CREATE or UPDATE operation on any `EtcdCluster` or `EtcdPeer` custom resource,
the Kubernetes API server will initiate an HTTPS webhook request to the `controller-manager`,
so that it can perform defaulting and validation of the resource before it is stored by the Kubernetes API server.

The API server connects to the webhook server using HTTPS
and this requires SSL certificates to be configured for the client and the server.
The easiest way to set this up is to [install Cert-Manager](https://book.kubebuilder.io/cronjob-tutorial/running-webhook.html#cert-manager)
before you deploy the Operator.
The `config/default/` directory contains Kustomize patches which add [Cert-Manager cainjector annotations](https://docs.cert-manager.io/en/latest/reference/cainjector.html) to the webhook configuration,
and a [self signing Issuer](https://docs.cert-manager.io/en/latest/tasks/issuers/setup-selfsigned.html).
With these, Cert-manager will automatically generate self-signed certificates for the webhook client and server.

## Operator

The Operator YAML is managed through [kustomize](https://github.com/kubernetes-sigs/kustomize). To install you require
 both the `kustomize` and `kubectl` binaries.
 
```bash
cd config/default
export ECO_VERSION=v0.2.0
kustomize edit set image controller=$ECO_VERSION
kustomize edit set image proxy=$ECO_VERSION
kubectl apply --kustomize .
```
