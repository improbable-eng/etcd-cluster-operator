# Nixery image registry generates images on the fly with the counted dependencies.
FROM nixery.dev/shell/remake/go_1_16/gcc/kustomize as build

ENV PATH=/share/go/bin:$PATH

ARG OPERATOR_IMAGE=storageos/etcd-cluster-operator-controller:develop
ARG PROXY_IMAGE=storageos/etcd-cluster-operator-proxy:develop

WORKDIR /tmp/src

COPY . .

RUN cd config/bases/manager && kustomize edit set image controller=${OPERATOR_IMAGE} && kustomize edit set image proxy=${PROXY_IMAGE}
RUN kustomize build config/default > etcd-cluster-operator.yaml
COPY config/samples/storageos-etcd-cluster.yaml storageos-etcd-cluster.yaml
# Create the final image.

FROM busybox:1.33.1

COPY --from=build /tmp/src/etcd-cluster-operator.yaml /etcd-operator.yaml
COPY --from=build /tmp/src/storageos-etcd-cluster.yaml /etcd-cluster.yaml

ENTRYPOINT cat /etcd-operator.yaml
