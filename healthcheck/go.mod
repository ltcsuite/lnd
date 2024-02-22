module github.com/ltcsuite/lnd/healthcheck

go 1.19

require (
	github.com/btcsuite/btclog v0.0.0-20170628155309-84c8d2346e9f
	github.com/ltcsuite/lnd/ticker v1.1.0
	github.com/ltcsuite/lnd/tor v1.0.0
	github.com/stretchr/testify v1.8.2
	golang.org/x/sys v0.8.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/klauspost/cpuid/v2 v2.0.9 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/ltcsuite/ltcd v0.23.5 // indirect
	github.com/ltcsuite/ltcd/chaincfg/chainhash v1.0.2 // indirect
	github.com/miekg/dns v1.1.43 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/crypto v0.7.0 // indirect
	golang.org/x/net v0.10.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	lukechampine.com/blake3 v1.2.1 // indirect
)

replace github.com/ltcsuite/lnd/ticker => ../ticker

replace github.com/ltcsuite/lnd/tor => ../tor
