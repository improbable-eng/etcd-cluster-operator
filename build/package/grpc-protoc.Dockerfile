# This Dockerfile is used to build an image with the Protobuf compiler and the Go Protobuf plugin together. This image
# is not published from this repository, but is used to generate our protobuf Go code. This is nessicary as we need to
# use spesific pinned versions of both protoc and the Go plugin to generate the correct output.

# Ubuntu is used to build protoc because the protoc build instructions give documtation for it
# https://github.com/protocolbuffers/protobuf/blob/master/src/README.md
FROM ubuntu:18.04 as protoc-builder

# Lovingly adapted from the unmainted (Apache 2.0 licensed) https://github.com/grpc/grpc-docker-library
RUN apt-get update && \
    apt-get install -y autoconf automake libtool curl make g++ unzip git
RUN git clone https://github.com/protocolbuffers/protobuf.git && \
    cd protobuf && \
    git checkout v3.11.3 && \
    git submodule update --init --recursive && \
    ./autogen.sh && \
    ./configure --prefix=/usr && \
    make && \
    make check && \
    make install && \
    ldconfig

FROM golang:1.13.1 as go-plugin-builder

# Get our version of the Go plugin
RUN go get -d -u github.com/golang/protobuf/protoc-gen-go
RUN git -C "$(go env GOPATH)"/src/github.com/golang/protobuf checkout v1.2.0
RUN go install github.com/golang/protobuf/protoc-gen-go

FROM gcr.io/distroless/static:nonroot as release
USER nonroot:nonroot

# Move the Go plugin to here
COPY --from=go-plugin-builder /go/bin/protoc-gen-go /bin/
COPY --from=protoc-builder /usr/bin/protoc /bin/
COPY --from=protoc-builder /usr/lib/google /lib/

ENTRYPOINT ["/bin/protoc"]
