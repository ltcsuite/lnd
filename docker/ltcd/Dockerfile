FROM golang:1.24.2-alpine as builder

LABEL maintainer="Loshan <loshan1212@gmail.com>"

# Install build dependencies such as git and glide.
# Grab and install the latest version of roasbeef's fork of ltcd and all

WORKDIR $GOPATH/src/github.com/ltcsuite/ltcd

# Pin down ltcd to a version that we know works with lnd.
ARG BTCD_VERSION=v0.23.4

# Grab and install the latest version of of ltcd and all related dependencies.
RUN git clone https://github.com/ltcsuite/ltcd.git . \
    && git checkout $BTCD_VERSION \
    && go install -v . ./cmd/...

# Start a new image
FROM alpine as final

# Expose mainnet ports (server, rpc)
EXPOSE 9333 9334

# Expose testnet ports (server, rpc)
EXPOSE 19334 19335

# Expose simnet ports (server, rpc)
EXPOSE 18555 18556

# Copy the compiled binaries from the builder image.
COPY --from=builder /go/bin/addblock /bin/
COPY --from=builder /go/bin/ltcctl /bin/
COPY --from=builder /go/bin/ltcd /bin/
COPY --from=builder /go/bin/findcheckpoint /bin/
COPY --from=builder /go/bin/gencerts /bin/

COPY "start-ltcctl.sh" .
COPY "start-ltcd.sh" .

RUN apk add --no-cache \
    bash \
    ca-certificates \
&&  mkdir "/rpc" "/root/.ltcd" "/root/.ltcctl" \
&&  touch "/root/.ltcd/ltcd.conf" \
&&  chmod +x start-ltcctl.sh \
&&  chmod +x start-ltcd.sh \
# Manually generate certificate and add all domains, it is needed to connect

# "ltcctl" and "lnd" to "ltcd" over docker links.
&&  "/bin/gencerts" --host="*" --directory="/rpc" --force

# Create a volume to house pregenerated RPC credentials. This will be
# shared with any lnd, btcctl containers so they can securely query ltcd's RPC
# server.
# You should NOT do this before certificate generation!
# Otherwise manually generated certificate will be overridden with shared
# mounted volume! For more info read dockerfile "VOLUME" documentation.
VOLUME ["/rpc"]
