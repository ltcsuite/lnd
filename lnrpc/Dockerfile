FROM swift:jammy

RUN apt-get update && apt-get install -y \
  git \
  protobuf-compiler \
  clang-format='1:14.0*'

RUN apt-get install -y wget \
    && ARCH=$(dpkg --print-architecture) \
    && if [ "$ARCH" = "arm64" ]; then \
         wget -c https://golang.org/dl/go1.24.2.linux-arm64.tar.gz -O - | tar -xz -C /usr/local; \
       else \
         wget -c https://golang.org/dl/go1.24.2.linux-amd64.tar.gz -O - | tar -xz -C /usr/local; \
       fi
ENV GOPATH=/go
ENV PATH=$PATH:/usr/local/go/bin:/go/bin

# We don't want any default values for these variables to make sure they're
# explicitly provided by parsing the go.mod file. Otherwise we might forget to
# update them here if we bump the versions.
ARG PROTOBUF_VERSION
ARG GRPC_GATEWAY_VERSION

ENV PROTOC_GEN_GO_GRPC_VERSION="v1.1.0"
ENV FALAFEL_VERSION="v0.0.0-cshared6"
ENV GOCACHE=/tmp/build/.cache
ENV GOMODCACHE=/tmp/build/.modcache

RUN cd /tmp \
  && mkdir -p /tmp/build/.cache \
  && mkdir -p /tmp/build/.modcache \
  && go install google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOBUF_VERSION} \
  && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@${PROTOC_GEN_GO_GRPC_VERSION} \
  && go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@${GRPC_GATEWAY_VERSION} \
  && go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@${GRPC_GATEWAY_VERSION} \
  && go install github.com/hsjoberg/falafel@${FALAFEL_VERSION} \
  && go install golang.org/x/tools/cmd/goimports@v0.1.7 \
  && chmod -R 777 /tmp/build/

WORKDIR /build

RUN git clone https://github.com/apple/swift-protobuf.git \
&& cd swift-protobuf \
&& git checkout $(git describe --tags --abbrev=0) \
&& git submodule update --init --recursive \
&& swift build -c release \
&& mv .build/release/protoc-gen-swift /bin

CMD ["/bin/bash", "/build/lnrpc/gen_protos.sh"]
