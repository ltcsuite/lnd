# Table of Contents
* [Installation](#installation)
  * [Installing a binary release](#installing-a-binary-release)
  * [Building a tagged version with Docker](#building-a-tagged-version-with-docker)
  * [Building a development version from source](#building-a-development-version-from-source)
    * [Preliminaries](#preliminaries-for-installing-from-source)
    * [Installing lnd](#installing-lnd-from-source)
* [Available Backend Operating Modes](#available-backend-operating-modes)
  * [ltcd Options](#ltcd-options)
  * [Neutrino Options](#neutrino-options)
  * [Litecoind Options](#litecoind-options)
  * [Using ltcd](#using-ltcd)
    * [Installing ltcd](#installing-ltcd)
    * [Starting ltcd](#starting-ltcd)
    * [Running lnd using the ltcd backend](#running-lnd-using-the-ltcd-backend)
  * [Using Neutrino](#using-neutrino)
  * [Using Litecoind](#using-litecoind)
* [Creating a Wallet](#creating-a-wallet)
* [Macaroons](#macaroons)
* [Network Reachability](#network-reachability)
* [Simnet vs. Testnet Development](#simnet-vs-testnet-development)
* [Creating an lnd.conf (Optional)](#creating-an-lndconf-optional)

# Installation

There are multiple ways to install `lnd`. For most users the easiest way is to
[download and install an official release binary](#installing-a-binary-release).
Those release binaries are always built with production in mind and have all
RPC subservers enabled.

More advanced users that want to build `lnd` from source also have multiple
options. To build a tagged version, there is a docker build helper script that
allows users to
[build `lnd` from source without needing to install `golang`](#building-a-tagged-version-with-docker).
That is also the preferred way to build and verify the reproducible builds that
are released by the team. See
[release.md for more information about reproducible builds](release.md).

Finally, there is the option to build `lnd` fully manually. This requires more
tooling to be set up first but allows to produce non-production (debug,
development) builds.

## Installing a binary release

Downloading and installing an official release binary is recommended for use on
mainnet.
[Visit the release page on GitHub](https://github.com/ltcsuite/lnd/releases)
and select the latest version that does not have the "Pre-release" label set
(unless you explicitly want to help test a Release Candidate, RC).

Choose the package that best fits your operating system and system architecture.
It is recommended to choose 64bit versions over 32bit ones, if your operating
system supports both.

Extract the package and place the two binaries (`lnd` and `lncli` or `lnd.exe`
and `lncli.exe` on Windows) somewhere where the operating system can find them.

## Building a tagged version with Docker

To use the Docker build helper, you need to have the following software
installed and set up on your machine:
 - Docker
 - `make`
 - `bash`

To build a specific git tag of `lnd`, simply run the following steps (assuming
`v0.x.y-beta` is the tagged version to build):

```shell
⛰  git clone https://github.com/ltcsuite/lnd
⛰  cd lnd
⛰  git checkout v0.x.y-beta
⛰  make docker-release tag=v0.x.y-beta
```

This will create a directory called `lnd-v0.x.y-beta` that contains the release
binaries for all operating system and architecture pairs. A single pair can also
be selected by specifying the `sys=linux-amd64` flag for example. See
[release.md for more information on reproducible builds](release.md).

## Building a development version from source

Building and installing `lnd` from source is only recommended for advanced users
and/or developers. Running the latest commit from the `master` branch is not
recommended for mainnet. The `master` branch can at times be unstable and
running your node off of it can prevent it to go back to a previous, stable
version if there are database migrations present.

### Preliminaries for installing from source
  In order to work with [`lnd`](https://github.com/ltcsuite/lnd), the
  following build dependencies are required:

  * **Go:** `lnd` is written in Go. To install, run one of the following commands:


    **Note**: The minimum version of Go supported is Go 1.16. We recommend that
    users use the latest version of Go, which at the time of writing is
    [`1.17.1`](https://blog.golang.org/go1.17.1).


    On Linux:

    (x86-64)
    ```
    wget https://dl.google.com/go/go1.17.1.linux-amd64.tar.gz
    sha256sum go1.17.1.linux-amd64.tar.gz | awk -F " " '{ print $1 }'
    ```

    The final output of the command above should be
    `dab7d9c34361dc21ec237d584590d72500652e7c909bf082758fb63064fca0ef`. If it
    isn't, then the target REPO HAS BEEN MODIFIED, and you shouldn't install
    this version of Go. If it matches, then proceed to install Go:
    ```
    sudo tar -C /usr/local -xzf go1.17.1.linux-amd64.tar.gz
    export PATH=$PATH:/usr/local/go/bin
    ```

    (ARMv6)
    ```
    wget https://dl.google.com/go/go1.17.1.linux-armv6l.tar.gz
    sha256sum go1.17.1.linux-armv6l.tar.gz | awk -F " " '{ print $1 }'
    ```

    The final output of the command above should be
    `ed3e4dbc9b80353f6482c441d65b51808290e94ff1d15d56da5f4a7be7353758`. If it
    isn't, then the target REPO HAS BEEN MODIFIED, and you shouldn't install
    this version of Go. If it matches, then proceed to install Go:
    ```
    tar -C /usr/local -xzf go1.17.1.linux-armv6l.tar.gz
    export PATH=$PATH:/usr/local/go/bin
    ```

    On Mac OS X:
    ```
    brew install go@1.17.1
    ```

    On FreeBSD:
    ```
    pkg install go
    ```

    Alternatively, one can download the pre-compiled binaries hosted on the
    [Golang download page](https://golang.org/dl/). If one seeks to install
    from source, then more detailed installation instructions can be found
    [here](https://golang.org/doc/install).

    At this point, you should set your `$GOPATH` environment variable, which
    represents the path to your workspace. By default, `$GOPATH` is set to
    `~/go`. You will also need to add `$GOPATH/bin` to your `PATH`. This ensures
    that your shell will be able to detect the binaries you install.

    ```shell
    ⛰  export GOPATH=~/gocode
    ⛰  export PATH=$PATH:$GOPATH/bin
    ```

    We recommend placing the above in your .bashrc or in a setup script so that
    you can avoid typing this every time you open a new terminal window.

  * **Go modules:** This project uses [Go modules](https://github.com/golang/go/wiki/Modules) 
    to manage dependencies as well as to provide *reproducible builds*.

    Usage of Go modules (with Go 1.13) means that you no longer need to clone
    `lnd` into your `$GOPATH` for development purposes. Instead, your `lnd`
    repo can now live anywhere!

### Installing lnd from source

With the preliminary steps completed, to install `lnd`, `lncli`, and all
related dependencies run the following commands:
```shell
⛰  git clone https://github.com/ltcsuite/lnd
⛰  cd lnd
⛰  make install
```

The command above will install the current _master_ branch of `lnd`. If you
wish to install a tagged release of `lnd` (as the master branch can at times be
unstable), then [visit then release page to locate the latest
release](https://github.com/ltcsuite/lnd/releases). Assuming the name
of the release is `v0.x.x`, then you can compile this release from source with
a small modification to the above command: 
```shell
⛰  git clone https://github.com/ltcsuite/lnd
⛰  cd lnd
⛰  git checkout v0.x.x
⛰  make install
```


**NOTE**: Our instructions still use the `$GOPATH` directory from prior
versions of Go, but with Go 1.13, it's now possible for `lnd` to live
_anywhere_ on your file system.

For Windows WSL users, make will need to be referenced directly via
/usr/bin/make/, or alternatively by wrapping quotation marks around make,
like so:

```shell
⛰  /usr/bin/make && /usr/bin/make install

⛰  "make" && "make" install
```

On FreeBSD, use gmake instead of make.

Alternatively, if one doesn't wish to use `make`, then the `go` commands can be
used directly:
```shell
⛰  go install -v ./...
```

**Updating**

To update your version of `lnd` to the latest version run the following
commands:
```shell
⛰  cd $GOPATH/src/github.com/ltcsuite/lnd
⛰  git pull
⛰  make clean && make && make install
```

On FreeBSD, use gmake instead of make.

Alternatively, if one doesn't wish to use `make`, then the `go` commands can be
used directly:
```shell
⛰  cd $GOPATH/src/github.com/ltcsuite/lnd
⛰  git pull
⛰  go install -v ./...
```

**Tests**

To check that `lnd` was installed properly run the following command:
```shell
⛰   make check
```

This command requires `litecoind` (almost any version should do) to be available
in the system's `$PATH` variable. Otherwise some of the tests will fail.

# Available Backend Operating Modes

In order to run, `lnd` requires, that the user specify a chain backend. At the
time of writing of this document, there are three available chain backends:
`ltcd`, `neutrino`, `litecoind`. All including neutrino can run on mainnet with
an out of the box `lnd` instance. We don't require `--txindex` when running
with `litecoind` or `ltcd` but activating the `txindex` will generally make
`lnd` run faster. Note that since version 0.13 pruned nodes are supported
although they cause performance penalty and higher network usage.

The set of arguments for each of the backend modes is as follows:

## ltcd Options
```text
ltcd:
      --ltcd.dir=                                             The base directory that contains the node's data, logs, configuration file, etc. (default: /Users/roasbeef/Library/Application Support/ltcd)
      --ltcd.rpchost=                                         The daemon's rpc listening address. If a port is omitted, then the default port for the selected chain parameters will be used. (default: localhost)
      --ltcd.rpcuser=                                         Username for RPC connections
      --ltcd.rpcpass=                                         Password for RPC connections
      --ltcd.rpccert=                                         File containing the daemon's certificate file (default: /Users/roasbeef/Library/Application Support/Ltcd/rpc.cert)
      --ltcd.rawrpccert=                                      The raw bytes of the daemon's PEM-encoded certificate chain which will be used to authenticate the RPC connection.
```

## Neutrino Options
```text
neutrino:
  -a, --neutrino.addpeer=                                     Add a peer to connect with at startup
      --neutrino.connect=                                     Connect only to the specified peers at startup
      --neutrino.maxpeers=                                    Max number of inbound and outbound peers
      --neutrino.banduration=                                 How long to ban misbehaving peers.  Valid time units are {s, m, h}.  Minimum 1 second
      --neutrino.banthreshold=                                Maximum allowed ban score before disconnecting and banning misbehaving peers.
      --neutrino.useragentname=                               Used to help identify ourselves to other litecoin peers.
      --neutrino.useragentversion=                            Used to help identify ourselves to other litecoin peers.
```

## litecoind Options
```text
litecoind:
      --litecoind.dir=                                         The base directory that contains the node's data, logs, configuration file, etc. (default: /Users/roasbeef/Library/Application Support/Litecoin)
      --litecoind.rpchost=                                     The daemon's rpc listening address. If a port is omitted, then the default port for the selected chain parameters will be used. (default: localhost)
      --litecoind.rpcuser=                                     Username for RPC connections
      --litecoind.rpcpass=                                     Password for RPC connections
      --litecoind.zmqpubrawblock=                              The address listening for ZMQ connections to deliver raw block notifications
      --litecoind.zmqpubrawtx=                                 The address listening for ZMQ connections to deliver raw transaction notifications
      --litecoind.estimatemode=                                The fee estimate mode. Must be either "ECONOMICAL" or "CONSERVATIVE". (default: CONSERVATIVE)
```

## Using ltcd

### Installing ltcd

On FreeBSD, use gmake instead of make.

To install ltcd, run the following commands:

Install **ltcd**:
```shell
⛰   make ltcd
```

Alternatively, you can install [`ltcd` directly from its
repo](https://github.com/ltcsuite/ltcd).

### Starting ltcd

Running the following command will create `rpc.cert` and default `ltcd.conf`.

```shell
⛰   ltcd --testnet --rpcuser=REPLACEME --rpcpass=REPLACEME
```
If you want to use `lnd` on testnet, `ltcd` needs to first fully sync the
testnet blockchain. Depending on your hardware, this may take up to a few
hours. Note that adding `--txindex` is optional, as it will take longer to sync
the node, but then `lnd` will generally operate faster as it can hit the index
directly, rather than scanning blocks or BIP 158 filters for relevant items.

(NOTE: It may take several minutes to find segwit-enabled peers.)

While `ltcd` is syncing you can check on its progress using ltcd's `getinfo`
RPC command:
```shell
⛰   btcctl --testnet --rpcuser=REPLACEME --rpcpass=REPLACEME getinfo
{
  "version": 120000,
  "protocolversion": 70002,
  "blocks": 1114996,
  "timeoffset": 0,
  "connections": 7,
  "proxy": "",
  "difficulty": 422570.58270815,
  "testnet": true,
  "relayfee": 0.00001,
  "errors": ""
}
```

Additionally, you can monitor ltcd's logs to track its syncing progress in real
time.

You can test your `ltcd` node's connectivity using the `getpeerinfo` command:
```shell
⛰   btcctl --testnet --rpcuser=REPLACEME --rpcpass=REPLACEME getpeerinfo | more
```

### Running lnd using the ltcd backend

If you are on testnet, run this command after `ltcd` has finished syncing.
Otherwise, replace `--litecoin.testnet` with `--litecoin.simnet`. If you are
installing `lnd` in preparation for the
[tutorial](https://dev.lightning.community/tutorial), you may skip this step.
```shell
⛰   lnd --litecoin.active --litecoin.testnet --debuglevel=debug \
       --ltcd.rpcuser=kek --ltcd.rpcpass=kek --externalip=X.X.X.X
```

## Using Neutrino

In order to run `lnd` in its light client mode, you'll need to locate a
full-node which is capable of serving this new light client mode. `lnd` uses
[BIP 157](https://github.com/bitcoin/bips/blob/master/bip-0157.mediawiki) and [BIP
158](https://github.com/bitcoin/bips/blob/master/bip-0158.mediawiki) for its light client
mode.  A public instance of such a node can be found at
`faucet.lightning.community`.

To run lnd in neutrino mode, run `lnd` with the following arguments, (swapping
in `--litecoin.simnet` if needed), and also your own `ltcd` node if available:
```shell
⛰   lnd --litecoin.active --litecoin.testnet --debuglevel=debug \
       --litecoin.node=neutrino --neutrino.connect=faucet.lightning.community
```


## Using litecoind

Note that adding `--txindex` is
optional, as it will take longer to sync the node, but then `lnd` will
generally operate faster as it can hit the index directly, rather than scanning
blocks or BIP 158 filters for relevant items.

To configure your litecoind backend for use with lnd, first complete and verify
the following:

- Since `lnd` uses
  [ZeroMQ](https://github.com/bitcoin/bitcoin/blob/master/doc/zmq.md) to
  interface with `litecoind`, *your `litecoind` installation must be compiled with
  ZMQ*. Note that if you installed `litecoind` from source and ZMQ was not present, 
  then ZMQ support will be disabled, and `lnd` will quit on a `connection refused` error. 
  If you installed `litecoind` via Homebrew in the past ZMQ may not be included 
  ([this has now been fixed](https://github.com/Homebrew/homebrew-core/pull/23088) 
  in the latest Homebrew recipe for litecoin)
- Configure the `litecoind` instance for ZMQ with `--zmqpubrawblock` and
  `--zmqpubrawtx`. These options must each use their own unique address in order
  to provide a reliable delivery of notifications (e.g.
  `--zmqpubrawblock=tcp://127.0.0.1:28332` and
  `--zmqpubrawtx=tcp://127.0.0.1:28333`).
- Start `litecoind` running against testnet, and let it complete a full sync with
  the testnet chain (alternatively, use `--litecoind.regtest` instead).

Here's a sample `litecoin.conf` for use with lnd:
```text
testnet=1
server=1
daemon=1
zmqpubrawblock=tcp://127.0.0.1:28332
zmqpubrawtx=tcp://127.0.0.1:28333
```

Once all of the above is complete, and you've confirmed `litecoind` is fully
updated with the latest blocks on testnet, run the command below to launch
`lnd` with `litecoind` as your backend (as with `litecoind`, you can create an
`lnd.conf` to save these options, more info on that is described further
below):

```shell
⛰   lnd --litecoin.active --litecoin.testnet --debuglevel=debug \
       --litecoin.node=litecoind --litecoind.rpcuser=REPLACEME \
       --litecoind.rpcpass=REPLACEME \
       --litecoind.zmqpubrawblock=tcp://127.0.0.1:28332 \
       --litecoind.zmqpubrawtx=tcp://127.0.0.1:28333 \
       --externalip=X.X.X.X
```

*NOTE:*
- The auth parameters `rpcuser` and `rpcpass` parameters can typically be
  determined by `lnd` for a `litecoind` instance running under the same user,
  including when using cookie auth. In this case, you can exclude them from the
  `lnd` options entirely.
- If you DO choose to explicitly pass the auth parameters in your `lnd.conf` or
  command line options for `lnd` (`litecoind.rpcuser` and `litecoind.rpcpass` as
  shown in example command above), you must also specify the
  `litecoind.zmqpubrawblock` and `litecoind.zmqpubrawtx` options. Otherwise, `lnd`
  will attempt to get the configuration from your `litecoin.conf`.
- You must ensure the same addresses are used for the `litecoind.zmqpubrawblock`
  and `litecoind.zmqpubrawtx` options passed to `lnd` as for the `zmqpubrawblock`
  and `zmqpubrawtx` passed in the `litecoind` options respectively.
- When running lnd and litecoind on the same Windows machine, ensure you use
  127.0.0.1, not localhost, for all configuration options that require a TCP/IP
  host address.  If you use "localhost" as the host name, you may see extremely
  slow inter-process-communication between lnd and the litecoind backend.  If lnd
  is experiencing this issue, you'll see "Waiting for chain backend to finish
  sync, start_height=XXXXXX" as the last entry in the console or log output, and
  lnd will appear to hang.  Normal lnd output will quickly show multiple
  messages like this as lnd consumes blocks from litecoind.
- Don't connect more than two or three instances of `lnd` to `litecoind`. With
  the default `litecoind` settings, having more than one instance of `lnd`, or
  `lnd` plus any application that consumes the RPC could cause `lnd` to miss
  crucial updates from the backend.
- The default fee estimate mode in `litecoind` is CONSERVATIVE. You can set
  `litecoind.estimatemode=ECONOMICAL` to change it into ECONOMICAL. Futhermore,
  if you start `litecoind` in `regtest`, this configuration won't take any effect.


# Creating a wallet
If `lnd` is being run for the first time, create a new wallet with:
```shell
⛰   lncli create
```
This will prompt for a wallet password, and optionally a cipher seed
passphrase.

`lnd` will then print a 24 word cipher seed mnemonic, which can be used to
recover the wallet in case of data loss. The user should write this down and
keep in a safe place.

More [information about managing wallets can be found in the wallet management
document](wallet.md).

# Macaroons

`lnd`'s authentication system is called **macaroons**, which are decentralized
bearer credentials allowing for delegation, attenuation, and other cool
features. You can learn more about them in Alex Akselrod's [writeup on
Github](https://github.com/ltcsuite/lnd/issues/20).

Running `lnd` for the first time will by default generate the `admin.macaroon`,
`read_only.macaroon`, and `macaroons.db` files that are used to authenticate
into `lnd`. They will be stored in the network directory (default:
`lnddir/data/chain/litecoin/mainnet`) so that it's possible to use a distinct
password for mainnet, testnet, simnet, etc. Note that if you specified an
alternative data directory (via the `--datadir` argument), you will have to
additionally pass the updated location of the `admin.macaroon` file into `lncli`
using the `--macaroonpath` argument.

To disable macaroons for testing, pass the `--no-macaroons` flag into *both*
`lnd` and `lncli`.

# Network Reachability

If you'd like to signal to other nodes on the network that you'll accept
incoming channels (as peers need to connect inbound to initiate a channel
funding workflow), then the `--externalip` flag should be set to your publicly
reachable IP address.

# Simnet vs. Testnet Development

If you are doing local development, such as for the tutorial, you'll want to
start both `ltcd` and `lnd` in the `simnet` mode. Simnet is similar to regtest
in that you'll be able to instantly mine blocks as needed to test `lnd`
locally. In order to start either daemon in the `simnet` mode use `simnet`
instead of `testnet`, adding the `--litecoin.simnet` flag instead of the
`--litecoin.testnet` flag.

Another relevant command line flag for local testing of new `lnd` developments
is the `--debughtlc` flag. When starting `lnd` with this flag, it'll be able to
automatically settle a special type of HTLC sent to it. This means that you
won't need to manually insert invoices in order to test payment connectivity.
To send this "special" HTLC type, include the `--debugsend` command at the end
of your `sendpayment` commands.


There are currently two primary ways to run `lnd`: one requires a local `ltcd`
instance with the RPC service exposed, and the other uses a fully integrated
light client powered by [neutrino](https://github.com/ltcsuite/neutrino).

# Creating an lnd.conf (Optional)

Optionally, if you'd like to have a persistent configuration between `lnd`
launches, allowing you to simply type `lnd --litecoin.testnet --litecoin.active`
at the command line, you can create an `lnd.conf`.

**On MacOS, located at:**
`/Users/[username]/Library/Application Support/Lndltc/lnd.conf`

**On Linux, located at:**
`~/.lndltc/lnd.conf`

Here's a sample `lnd.conf` for `ltcd` to get you started:
```text
[Application Options]
debuglevel=trace
maxpendingchannels=10

[Bitcoin]
litecoin.active=1
```

Notice the `[Bitcoin]` section. This section houses the parameters for the
Bitcoin chain. `lnd` also supports Litecoin testnet4 (but not both BTC and LTC
at the same time), so when working with Litecoin be sure to set to parameters
for Litecoin accordingly. See a more detailed sample config file available
[here](https://github.com/ltcsuite/lnd/blob/master/sample-lnd.conf)
and explore the other sections for node configuration, including `[ltcd]`,
`[litecoind]`, `[Neutrino]`, `[Ltcd]`, and `[Litecoind]` depending on which
chain and node type you're using.
