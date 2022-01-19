# Nixery image registry generates images on the fly with the counted dependencies.
FROM nixery.dev/shell/remake/go_1_16/gcc/kustomize as build

ENV PATH=/share/go/bin:$PATH

WORKDIR /tmp/src

COPY . .

RUN kustomize build config/default > etcd-cluster-operator.yaml

# Create the final image.

FROM busybox:1.33.1

COPY --from=build /tmp/src/etcd-cluster-operator.yaml /operator.yaml

ENTRYPOINT cat /operator.yaml
