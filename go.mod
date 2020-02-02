module github.com/ltcsuite/lnd

require (
	git.schwanenlied.me/yawning/bsaes.git v0.0.0-20180720073208-c0276d75487e // indirect
	github.com/NebulousLabs/fastrand v0.0.0-20181203155948-6fb6489aac4e // indirect
	github.com/NebulousLabs/go-upnp v0.0.0-20180202185039-29b680b06c82
	github.com/Yawning/aez v0.0.0-20180114000226-4dad034d9db2
	github.com/btcsuite/btclog v0.0.0-20170628155309-84c8d2346e9f
	github.com/btcsuite/fastsha256 v0.0.0-20160815193821-637e65642941
	github.com/coreos/bbolt v1.3.3
	github.com/davecgh/go-spew v1.1.1
	github.com/go-errors/errors v1.0.1
	github.com/golang/protobuf v1.3.3
	github.com/grpc-ecosystem/go-grpc-middleware v1.0.0
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/grpc-ecosystem/grpc-gateway v1.12.2
	github.com/jackpal/gateway v1.0.5
	github.com/jackpal/go-nat-pmp v0.0.0-20170405195558-28a68d0c24ad
	github.com/jessevdk/go-flags v1.4.0
	github.com/jrick/logrotate v1.0.0
	github.com/juju/clock v0.0.0-20190205081909-9c5c9712527c // indirect
	github.com/juju/errors v0.0.0-20190806202954-0232dcc7464d // indirect
	github.com/juju/loggo v0.0.0-20190526231331-6e530bcce5d8 // indirect
	github.com/juju/retry v0.0.0-20180821225755-9058e192b216 // indirect
	github.com/juju/testing v0.0.0-20190723135506-ce30eb24acd2 // indirect
	github.com/juju/utils v0.0.0-20180820210520-bf9cc5bdd62d // indirect
	github.com/juju/version v0.0.0-20180108022336-b64dbd566305 // indirect
	github.com/kkdai/bstream v1.0.0
	github.com/lightninglabs/falafel v0.0.0-20200121195531-41fd7b05eb59 // indirect
	github.com/lightninglabs/protobuf-hex-display v1.3.3-0.20191212020323-b444784ce75d
	github.com/lightningnetwork/lnd/queue v1.0.2 // indirect
	github.com/ltcsuite/lightning-onion v0.0.0-00010101000000-000000000000
	github.com/ltcsuite/lnd/cert v0.0.0-00010101000000-000000000000
	github.com/ltcsuite/lnd/queue v1.0.1
	github.com/ltcsuite/lnd/ticker v1.0.0
	github.com/ltcsuite/ltcd v0.20.1-beta
	github.com/ltcsuite/ltcutil v0.0.0-20191227053721-6bec450ea6ad
	github.com/ltcsuite/ltcwallet v0.11.1-beta
	github.com/ltcsuite/ltcwallet/wallet/txauthor v1.0.0
	github.com/ltcsuite/ltcwallet/wallet/txrules v1.0.0
	github.com/ltcsuite/ltcwallet/walletdb v1.2.0
	github.com/ltcsuite/ltcwallet/wtxmgr v1.0.0
	github.com/ltcsuite/neutrino v0.11.0
	github.com/miekg/dns v0.0.0-20171125082028-79bfde677fa8
	github.com/prometheus/client_golang v0.9.3
	github.com/tv42/zbase32 v0.0.0-20160707012821-501572607d02
	github.com/urfave/cli v1.18.0
	golang.org/x/crypto v0.0.0-20200128174031-69ecbb4d6d5d
	golang.org/x/net v0.0.0-20191002035440-2ec189313ef0
	golang.org/x/time v0.0.0-20180412165947-fbb02b2291d2
	google.golang.org/genproto v0.0.0-20200128133413-58ce757ed39b
	google.golang.org/grpc v1.24.0
	gopkg.in/errgo.v1 v1.0.1 // indirect
	gopkg.in/macaroon-bakery.v2 v2.0.1
	gopkg.in/macaroon.v2 v2.0.0
	gopkg.in/mgo.v2 v2.0.0-20190816093944-a6b53ec6cb22 // indirect
	gopkg.in/resty.v1 v1.12.0 // indirect
	gopkg.in/yaml.v2 v2.2.8 // indirect
)

replace github.com/ltcsuite/lnd/ticker => ./ticker

replace github.com/ltcsuite/lnd/queue => ./queue

replace github.com/ltcsuite/lnd/cert => ./cert

replace github.com/ltcsuite/lightning-onion => ../lightning-onion

replace git.schwanenlied.me/yawning/bsaes.git => github.com/Yawning/bsaes v0.0.0-20180720073208-c0276d75487e

replace github.com/ltcsuite/ltcwallet => ../../ltcsuite/ltcwallet

replace github.com/ltcsuite/ltcwallet/walletdb => ../../ltcsuite/ltcwallet/walletdb

replace github.com/ltcsuite/ltcwallet/wtxmgr => ../../ltcsuite/ltcwallet/wtxmgr

replace github.com/ltcsuite/ltcwallet/wallet/txauthor => ../../ltcsuite/ltcwallet/wallet/txauthor

replace github.com/ltcsuite/ltcwallet/wallet/txrules => ../../ltcsuite/ltcwallet/wallet/txrules

replace github.com/ltcsuite/ltcwallet/wallet/txsizes => ../../ltcsuite/ltcwallet/wallet/txsizes

replace github.com/ltcsuite/neutrino => ../../ltcsuite/neutrino

go 1.12
