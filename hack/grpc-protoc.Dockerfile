# This Dockerfile is used to build an image with the Protobuf compiler and the Go Protobuf plugin together. This image
# is not published from this repository, but is used to generate our protobuf Go code. This is necessary as we need to
# use specific pinned versions of both protoc and the Go plugin to generate the correct output.
ARG GO_VERSION
FROM golang:${GO_VERSION} as go-plugin-builder

RUN go get -d -u github.com/golang/protobuf/protoc-gen-go
RUN git -C "$(go env GOPATH)"/src/github.com/golang/protobuf checkout v1.3.2
RUN go install github.com/golang/protobuf/protoc-gen-go

FROM ubuntu:18.04

RUN apt-get update && apt-get install -y unzip
ADD https://github.com/protocolbuffers/protobuf/releases/download/v3.11.3/protoc-3.11.3-linux-x86_64.zip /tmp/
RUN unzip /tmp/protoc-3.11.3-linux-x86_64.zip -d /usr/local && \
    rm /tmp/protoc-3.11.3-linux-x86_64.zip && \
    rm /usr/local/readme.txt
COPY --from=go-plugin-builder /go/bin/protoc-gen-go /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/protoc"]
