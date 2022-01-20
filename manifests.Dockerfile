# Nixery image registry generates images on the fly with the counted dependencies.
FROM nixery.dev/shell/remake/go_1_16/gcc/kustomize as build

ENV PATH=/share/go/bin:$PATH

WORKDIR /tmp/src

COPY . .

RUN kustomize build config/default > etcd-cluster-operator.yaml
COPY config/samples/storageos-etcd-cluster.yaml storageos-etcd-cluster.yaml
# Create the final image.

FROM busybox:1.33.1

COPY --from=build /tmp/src/etcd-cluster-operator.yaml /operator.yaml
COPY --from=build /tmp/src/storageos-etcd-cluster.yaml /storageos-etcd-cluster.yaml

ENTRYPOINT cat /operator.yaml
