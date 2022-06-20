#!/usr/bin/env bash

set -ev

BITCOIND_VERSION=${BITCOIN_VERSION:-0.21.2}

docker pull litecoin-project/litecoin-core:$BITCOIND_VERSION
CONTAINER_ID=$(docker create litecoin-project/litecoin-core:$BITCOIND_VERSION)
sudo docker cp $CONTAINER_ID:/opt/litecoin-$BITCOIND_VERSION/bin/litecoind /usr/local/bin/litecoind
docker rm $CONTAINER_ID
