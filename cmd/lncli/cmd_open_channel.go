package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/ltcsuite/lnd/lnrpc"
	"github.com/ltcsuite/lnd/lnwallet/chanfunding"
	"github.com/ltcsuite/ltcd/chaincfg/chainhash"
	"github.com/ltcsuite/ltcd/ltcutil"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/urfave/cli"
)

const (
	userMsgFund = `PSBT funding initiated with peer %x.
Please create a PSBT that sends %v (%d satoshi) to the funding address %s.

Note: The whole process should be completed within 10 minutes, otherwise there
is a risk of the remote node timing out and canceling the funding process.

Example with bitcoind:
	bitcoin-cli walletcreatefundedpsbt [] '[{"%s":%.8f}]'

If you are using a wallet that can fund a PSBT directly (currently not possible
with bitcoind), you can use this PSBT that contains the same address and amount:
%s

!!! WARNING !!!
DO NOT PUBLISH the finished transaction by yourself or with another tool.
lnd MUST publish it in the proper funding flow order OR THE FUNDS CAN BE LOST!

Paste the funded PSBT here to continue the funding flow.
If your PSBT is very long (specifically, more than 4096 characters), please save
it to a file and paste the full file path here instead as some terminals will
truncate the pasted text if it's too long.
Base64 encoded PSBT (or path to file): `

	userMsgSign = `
PSBT verified by lnd, please continue the funding flow by signing the PSBT by
all required parties/devices. Once the transaction is fully signed, paste it
again here either in base64 PSBT or hex encoded raw wire TX format.

Signed base64 encoded PSBT or hex encoded raw wire TX (or path to file): `

	// psbtMaxFileSize is the maximum file size we allow a PSBT file to be
	// in case we want to read a PSBT from a file. This is mainly to protect
	// the user from choosing a large file by accident and running into out
	// of memory issues or other weird errors.
	psbtMaxFileSize = 1024 * 1024

	channelTypeTweakless     = "tweakless"
	channelTypeAnchors       = "anchors"
	channelTypeSimpleTaproot = "taproot"
)

// TODO(roasbeef): change default number of confirmations.
var openChannelCommand = cli.Command{
	Name:     "openchannel",
	Category: "Channels",
	Usage:    "Open a channel to a node or an existing peer.",
	Description: `
	Attempt to open a new channel to an existing peer with the key node-key
	optionally blocking until the channel is 'open'.

	One can also connect to a node before opening a new channel to it by
	setting its host:port via the --connect argument. For this to work,
	the node_key must be provided, rather than the peer_id. This is
	optional.

	The channel will be initialized with local-amt satoshis locally and
	push-amt satoshis for the remote node. Note that the push-amt is
	deducted from the specified local-amt which implies that the local-amt
	must be greater than the push-amt. Also note that specifying push-amt
	means you give that amount to the remote node as part of the channel
	opening. Once the channel is open, a channelPoint (txid:vout) of the
	funding output is returned.

	If the remote peer supports the option upfront shutdown feature bit
	(query listpeers to see their supported feature bits), an address to
	enforce payout of funds on cooperative close can optionally be provided.
	Note that if you set this value, you will not be able to cooperatively
	close out to another address.

	One can manually set the fee to be used for the funding transaction via
	either the --conf_target or --sat_per_vbyte arguments. This is
	optional.

	One can also specify a short string memo to record some useful
	information about the channel using the --memo argument. This is stored
	locally only, and is purely for reference. It has no bearing on the
	channel's operation. Max allowed length is 500 characters.`,
	ArgsUsage: "node-key local-amt push-amt",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name: "node_key",
			Usage: "the identity public key of the target " +
				"node/peer serialized in compressed format",
		},
		cli.StringFlag{
			Name:  "connect",
			Usage: "(optional) the host:port of the target node",
		},
		cli.IntFlag{
			Name: "local_amt",
			Usage: "the number of satoshis the wallet should " +
				"commit to the channel",
		},
		cli.BoolFlag{
			Name: "fundmax",
			Usage: "if set, the wallet will attempt to commit " +
				"the maximum possible local amount to the " +
				"channel. This must not be set at the same " +
				"time as local_amt",
		},
		cli.StringSliceFlag{
			Name: "utxo",
			Usage: "a utxo specified as outpoint(tx:idx) which " +
				"will be used to fund a channel. This flag " +
				"can be repeatedly used to fund a channel " +
				"with a selection of utxos. The selected " +
				"funds can either be entirely spent by " +
				"specifying the fundmax flag or partially by " +
				"selecting a fraction of the sum of the " +
				"outpoints in local_amt",
		},
		cli.Uint64Flag{
			Name: "base_fee_msat",
			Usage: "the base fee in milli-satoshis that will " +
				"be charged for each forwarded HTLC, " +
				"regardless of payment size",
		},
		cli.Uint64Flag{
			Name: "fee_rate_ppm",
			Usage: "the fee rate ppm (parts per million) that " +
				"will be charged proportionally based on the " +
				"value of each forwarded HTLC, the lowest " +
				"possible rate is 0 with a granularity of " +
				"0.000001 (millionths)",
		},
		cli.IntFlag{
			Name: "push_amt",
			Usage: "the number of satoshis to give the remote " +
				"side as part of the initial commitment " +
				"state, this is equivalent to first opening " +
				"a channel and sending the remote party " +
				"funds, but done all in one step",
		},
		cli.BoolFlag{
			Name:  "block",
			Usage: "block and wait until the channel is fully open",
		},
		cli.Int64Flag{
			Name: "conf_target",
			Usage: "(optional) the number of blocks that the " +
				"transaction *should* confirm in, will be " +
				"used for fee estimation",
		},
		cli.Int64Flag{
			Name:   "sat_per_byte",
			Usage:  "Deprecated, use sat_per_vbyte instead.",
			Hidden: true,
		},
		cli.Int64Flag{
			Name: "sat_per_vbyte",
			Usage: "(optional) a manual fee expressed in " +
				"sat/vbyte that should be used when crafting " +
				"the transaction",
		},
		cli.BoolFlag{
			Name: "private",
			Usage: "make the channel private, such that it won't " +
				"be announced to the greater network, and " +
				"nodes other than the two channel endpoints " +
				"must be explicitly told about it to be able " +
				"to route through it",
		},
		cli.Int64Flag{
			Name: "min_htlc_msat",
			Usage: "(optional) the minimum value we will require " +
				"for incoming HTLCs on the channel",
		},
		cli.Uint64Flag{
			Name: "remote_csv_delay",
			Usage: "(optional) the number of blocks we will " +
				"require our channel counterparty to wait " +
				"before accessing its funds in case of " +
				"unilateral close. If this is not set, we " +
				"will scale the value according to the " +
				"channel size",
		},
		cli.Uint64Flag{
			Name: "max_local_csv",
			Usage: "(optional) the maximum number of blocks that " +
				"we will allow the remote peer to require we " +
				"wait before accessing our funds in the case " +
				"of a unilateral close.",
		},
		cli.Uint64Flag{
			Name: "min_confs",
			Usage: "(optional) the minimum number of " +
				"confirmations each one of your outputs used " +
				"for the funding transaction must satisfy",
			Value: defaultUtxoMinConf,
		},
		cli.StringFlag{
			Name: "close_address",
			Usage: "(optional) an address to enforce payout of " +
				"our funds to on cooperative close. Note " +
				"that if this value is set on channel open, " +
				"you will *not* be able to cooperatively " +
				"close to a different address.",
		},
		cli.BoolFlag{
			Name: "psbt",
			Usage: "start an interactive mode that initiates " +
				"funding through a partially signed bitcoin " +
				"transaction (PSBT), allowing the channel " +
				"funds to be added and signed from a " +
				"hardware or other offline device.",
		},
		cli.StringFlag{
			Name: "base_psbt",
			Usage: "when using the interactive PSBT mode to open " +
				"a new channel, use this base64 encoded PSBT " +
				"as a base and add the new channel output to " +
				"it instead of creating a new, empty one.",
		},
		cli.BoolFlag{
			Name: "no_publish",
			Usage: "when using the interactive PSBT mode to open " +
				"multiple channels in a batch, this flag " +
				"instructs lnd to not publish the full batch " +
				"transaction just yet. For safety reasons " +
				"this flag should be set for each of the " +
				"batch's transactions except the very last",
		},
		cli.Uint64Flag{
			Name: "remote_max_value_in_flight_msat",
			Usage: "(optional) the maximum value in msat that " +
				"can be pending within the channel at any " +
				"given time",
		},
		cli.StringFlag{
			Name: "channel_type",
			Usage: fmt.Sprintf("(optional) the type of channel to "+
				"propose to the remote peer (%q, %q, %q)",
				channelTypeTweakless, channelTypeAnchors,
				channelTypeSimpleTaproot),
		},
		cli.BoolFlag{
			Name: "zero_conf",
			Usage: "(optional) whether a zero-conf channel open " +
				"should be attempted.",
		},
		cli.BoolFlag{
			Name: "scid_alias",
			Usage: "(optional) whether a scid-alias channel type" +
				" should be negotiated.",
		},
		cli.Uint64Flag{
			Name: "remote_reserve_sats",
			Usage: "(optional) the minimum number of satoshis we " +
				"require the remote node to keep as a direct " +
				"payment. If not specified, a default of 1% " +
				"of the channel capacity will be used.",
		},
		cli.StringFlag{
			Name: "memo",
			Usage: `(optional) a note-to-self containing some useful
				information about the channel. This is stored
				locally only, and is purely for reference. It
				has no bearing on the channel's operation. Max
				allowed length is 500 characters`,
		},
	},
	Action: actionDecorator(openChannel),
}

func openChannel(ctx *cli.Context) error {
	// TODO(roasbeef): add deadline to context
	ctxc := getContext()
	client, cleanUp := getClient(ctx)
	defer cleanUp()

	args := ctx.Args()
	var err error

	// Show command help if no arguments provided
	if ctx.NArg() == 0 && ctx.NumFlags() == 0 {
		_ = cli.ShowCommandHelp(ctx, "openchannel")
		return nil
	}

	// Check that only the field sat_per_vbyte or the deprecated field
	// sat_per_byte is used.
	feeRateFlag, err := checkNotBothSet(
		ctx, "sat_per_vbyte", "sat_per_byte",
	)
	if err != nil {
		return err
	}

	minConfs := int32(ctx.Uint64("min_confs"))
	req := &lnrpc.OpenChannelRequest{
		TargetConf:                 int32(ctx.Int64("conf_target")),
		SatPerVbyte:                ctx.Uint64(feeRateFlag),
		MinHtlcMsat:                ctx.Int64("min_htlc_msat"),
		RemoteCsvDelay:             uint32(ctx.Uint64("remote_csv_delay")),
		MinConfs:                   minConfs,
		SpendUnconfirmed:           minConfs == 0,
		CloseAddress:               ctx.String("close_address"),
		RemoteMaxValueInFlightMsat: ctx.Uint64("remote_max_value_in_flight_msat"),
		MaxLocalCsv:                uint32(ctx.Uint64("max_local_csv")),
		ZeroConf:                   ctx.Bool("zero_conf"),
		ScidAlias:                  ctx.Bool("scid_alias"),
		RemoteChanReserveSat:       ctx.Uint64("remote_reserve_sats"),
		FundMax:                    ctx.Bool("fundmax"),
		Memo:                       ctx.String("memo"),
	}

	switch {
	case ctx.IsSet("node_key"):
		nodePubHex, err := hex.DecodeString(ctx.String("node_key"))
		if err != nil {
			return fmt.Errorf("unable to decode node public key: "+
				"%v", err)
		}
		req.NodePubkey = nodePubHex

	case args.Present():
		nodePubHex, err := hex.DecodeString(args.First())
		if err != nil {
			return fmt.Errorf("unable to decode node public key: "+
				"%v", err)
		}
		args = args.Tail()
		req.NodePubkey = nodePubHex
	default:
		return fmt.Errorf("node id argument missing")
	}

	// As soon as we can confirm that the node's node_key was set, rather
	// than the peer_id, we can check if the host:port was also set to
	// connect to it before opening the channel.
	if req.NodePubkey != nil && ctx.IsSet("connect") {
		addr := &lnrpc.LightningAddress{
			Pubkey: hex.EncodeToString(req.NodePubkey),
			Host:   ctx.String("connect"),
		}

		req := &lnrpc.ConnectPeerRequest{
			Addr: addr,
			Perm: false,
		}

		// Check if connecting to the node was successful.
		// We discard the peer id returned as it is not needed.
		_, err := client.ConnectPeer(ctxc, req)
		if err != nil &&
			!strings.Contains(err.Error(), "already connected") {

			return err
		}
	}

	switch {
	case ctx.IsSet("local_amt"):
		req.LocalFundingAmount = int64(ctx.Int("local_amt"))
	case args.Present():
		req.LocalFundingAmount, err = strconv.ParseInt(
			args.First(), 10, 64,
		)
		if err != nil {
			return fmt.Errorf("unable to decode local amt: %v", err)
		}
		args = args.Tail()
	case !ctx.Bool("fundmax"):
		return fmt.Errorf("either local_amt or fundmax must be " +
			"specified")
	}

	// The fundmax flag is NOT allowed to be combined with local_amt above.
	// It is allowed to be combined with push_amt, but only if explicitly
	// set.
	if ctx.Bool("fundmax") && req.LocalFundingAmount != 0 {
		return fmt.Errorf("local amount cannot be set if attempting " +
			"to commit the maximum amount out of the wallet")
	}

	// The fundmax flag is NOT allowed to be combined with the psbt flag.
	if ctx.Bool("fundmax") && ctx.Bool("psbt") {
		return fmt.Errorf("psbt cannot be set if attempting " +
			"to commit the maximum amount out of the wallet")
	}

	if ctx.IsSet("utxo") {
		utxos := ctx.StringSlice("utxo")

		outpoints, err := UtxosToOutpoints(utxos)
		if err != nil {
			return fmt.Errorf("unable to decode utxos: %w", err)
		}

		req.Outpoints = outpoints
	}

	if ctx.IsSet("push_amt") {
		req.PushSat = int64(ctx.Int("push_amt"))
	} else if args.Present() {
		req.PushSat, err = strconv.ParseInt(args.First(), 10, 64)
		if err != nil {
			return fmt.Errorf("unable to decode push amt: %v", err)
		}
	}

	if ctx.IsSet("base_fee_msat") {
		req.BaseFee = ctx.Uint64("base_fee_msat")
		req.UseBaseFee = true
	}

	if ctx.IsSet("fee_rate_ppm") {
		req.FeeRate = ctx.Uint64("fee_rate_ppm")
		req.UseFeeRate = true
	}

	req.Private = ctx.Bool("private")

	// Parse the channel type and map it to its RPC representation.
	channelType := ctx.String("channel_type")
	switch channelType {
	case "":
		break
	case channelTypeTweakless:
		req.CommitmentType = lnrpc.CommitmentType_STATIC_REMOTE_KEY
	case channelTypeAnchors:
		req.CommitmentType = lnrpc.CommitmentType_ANCHORS
	case channelTypeSimpleTaproot:
		req.CommitmentType = lnrpc.CommitmentType_SIMPLE_TAPROOT
	default:
		return fmt.Errorf("unsupported channel type %v", channelType)
	}

	// PSBT funding is a more involved, interactive process that is too
	// large to also fit into this already long function.
	if ctx.Bool("psbt") {
		return openChannelPsbt(ctxc, ctx, client, req)
	}
	if !ctx.Bool("psbt") && ctx.Bool("no_publish") {
		return fmt.Errorf("the --no_publish flag can only be used in " +
			"combination with the --psbt flag")
	}

	stream, err := client.OpenChannel(ctxc, req)
	if err != nil {
		return err
	}

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}

		switch update := resp.Update.(type) {
		case *lnrpc.OpenStatusUpdate_ChanPending:
			err := printChanPending(update)
			if err != nil {
				return err
			}

			if !ctx.Bool("block") {
				return nil
			}

		case *lnrpc.OpenStatusUpdate_ChanOpen:
			return printChanOpen(update)
		}
	}
}

// openChannelPsbt starts an interactive channel open protocol that uses a
// partially signed bitcoin transaction (PSBT) to fund the channel output. The
// protocol involves several steps between the RPC server and the CLI client:
//
// RPC server                           CLI client
//
//	|                                    |
//	|  |<------open channel (stream)-----|
//	|  |-------ready for funding----->|  |
//	|  |<------PSBT verify------------|  |
//	|  |-------ready for signing----->|  |
//	|  |<------PSBT finalize----------|  |
//	|  |-------channel pending------->|  |
//	|  |-------channel open------------->|
//	|                                    |
func openChannelPsbt(rpcCtx context.Context, ctx *cli.Context,
	client lnrpc.LightningClient,
	req *lnrpc.OpenChannelRequest) error {

	var (
		pendingChanID [32]byte
		shimPending   = true
		basePsbtBytes []byte
		quit          = make(chan struct{})
		srvMsg        = make(chan *lnrpc.OpenStatusUpdate, 1)
		srvErr        = make(chan error, 1)
		ctxc, cancel  = context.WithCancel(rpcCtx)
	)
	defer cancel()

	// Make sure the user didn't supply any command line flags that are
	// incompatible with PSBT funding.
	err := checkPsbtFlags(req)
	if err != nil {
		return err
	}

	// If the user supplied a base PSBT, only make sure it's valid base64.
	// The RPC server will make sure it's also a valid PSBT.
	basePsbt := ctx.String("base_psbt")
	if basePsbt != "" {
		basePsbtBytes, err = base64.StdEncoding.DecodeString(basePsbt)
		if err != nil {
			return fmt.Errorf("error parsing base PSBT: %v", err)
		}
	}

	// Generate a new, random pending channel ID that we'll use as the main
	// identifier when sending update messages to the RPC server.
	if _, err := rand.Read(pendingChanID[:]); err != nil {
		return fmt.Errorf("unable to generate random chan ID: %v", err)
	}
	fmt.Printf("Starting PSBT funding flow with pending channel ID %x.\n",
		pendingChanID)

	// maybeCancelShim is a helper function that cancels the funding shim
	// with the RPC server in case we end up aborting early.
	maybeCancelShim := func() {
		// If the user canceled while there was still a shim registered
		// with the wallet, release the resources now.
		if shimPending {
			fmt.Printf("Canceling PSBT funding flow for pending "+
				"channel ID %x.\n", pendingChanID)
			cancelMsg := &lnrpc.FundingTransitionMsg{
				Trigger: &lnrpc.FundingTransitionMsg_ShimCancel{
					ShimCancel: &lnrpc.FundingShimCancel{
						PendingChanId: pendingChanID[:],
					},
				},
			}
			err := sendFundingState(ctxc, ctx, cancelMsg)
			if err != nil {
				fmt.Printf("Error canceling shim: %v\n", err)
			}
			shimPending = false
		}

		// Abort the stream connection to the server.
		cancel()
	}
	defer maybeCancelShim()

	// Create the PSBT funding shim that will tell the funding manager we
	// want to use a PSBT.
	req.FundingShim = &lnrpc.FundingShim{
		Shim: &lnrpc.FundingShim_PsbtShim{
			PsbtShim: &lnrpc.PsbtShim{
				PendingChanId: pendingChanID[:],
				BasePsbt:      basePsbtBytes,
				NoPublish:     ctx.Bool("no_publish"),
			},
		},
	}

	// Start the interactive process by opening the stream connection to the
	// daemon. If the user cancels by pressing <Ctrl+C> we need to cancel
	// the shim. To not just kill the process on interrupt, we need to
	// explicitly capture the signal.
	stream, err := client.OpenChannel(ctxc, req)
	if err != nil {
		return fmt.Errorf("opening stream to server failed: %v", err)
	}

	// We also need to spawn a goroutine that reads from the server. This
	// will copy the messages to the channel as long as they come in or add
	// exactly one error to the error stream and then bail out.
	go func() {
		for {
			// Recv blocks until a message or error arrives.
			resp, err := stream.Recv()
			if err == io.EOF {
				srvErr <- fmt.Errorf("lnd shutting down: %v",
					err)
				return
			} else if err != nil {
				srvErr <- fmt.Errorf("got error from server: "+
					"%v", err)
				return
			}

			// Don't block on sending in case of shutting down.
			select {
			case srvMsg <- resp:
			case <-quit:
				return
			}
		}
	}()

	// Spawn another goroutine that only handles abort from user or errors
	// from the server. Both will trigger an attempt to cancel the shim with
	// the server.
	go func() {
		select {
		case <-rpcCtx.Done():
			fmt.Printf("\nInterrupt signal received.\n")
			close(quit)

		case err := <-srvErr:
			fmt.Printf("\nError received: %v\n", err)

			// If the remote peer canceled on us, the reservation
			// has already been deleted. We don't need to try to
			// remove it again, this would just produce another
			// error.
			cancelErr := chanfunding.ErrRemoteCanceled.Error()
			if err != nil && strings.Contains(
				err.Error(), cancelErr,
			) {

				shimPending = false
			}
			close(quit)

		case <-quit:
		}
	}()

	// Our main event loop where we wait for triggers
	for {
		var srvResponse *lnrpc.OpenStatusUpdate
		select {
		case srvResponse = <-srvMsg:
		case <-quit:
			return nil
		}

		switch update := srvResponse.Update.(type) {
		case *lnrpc.OpenStatusUpdate_PsbtFund:
			// First tell the user how to create the PSBT with the
			// address and amount we now know.
			amt := ltcutil.Amount(update.PsbtFund.FundingAmount)
			addr := update.PsbtFund.FundingAddress
			fmt.Printf(
				userMsgFund, req.NodePubkey, amt, amt, addr,
				addr, amt.ToBTC(),
				base64.StdEncoding.EncodeToString(
					update.PsbtFund.Psbt,
				),
			)

			// Read the user's response and send it to the server to
			// verify everything's correct before anything is
			// signed.
			inputPsbt, err := readTerminalOrFile(quit)
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("reading from terminal or "+
					"file failed: %v", err)
			}
			fundedPsbt, err := decodePsbt(inputPsbt)
			if err != nil {
				return fmt.Errorf("psbt decode failed: %v",
					err)
			}
			verifyMsg := &lnrpc.FundingTransitionMsg{
				Trigger: &lnrpc.FundingTransitionMsg_PsbtVerify{
					PsbtVerify: &lnrpc.FundingPsbtVerify{
						FundedPsbt:    fundedPsbt,
						PendingChanId: pendingChanID[:],
					},
				},
			}
			err = sendFundingState(ctxc, ctx, verifyMsg)
			if err != nil {
				return fmt.Errorf("verifying PSBT by lnd "+
					"failed: %v", err)
			}

			// Now that we know the PSBT looks good, we can let it
			// be signed by the user.
			fmt.Print(userMsgSign)

			// Read the signed PSBT and send it to lnd.
			finalTxStr, err := readTerminalOrFile(quit)
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("reading from terminal or "+
					"file failed: %v", err)
			}
			finalizeMsg, err := finalizeMsgFromString(
				finalTxStr, pendingChanID[:],
			)
			if err != nil {
				return err
			}
			transitionMsg := &lnrpc.FundingTransitionMsg{
				Trigger: finalizeMsg,
			}
			err = sendFundingState(ctxc, ctx, transitionMsg)
			if err != nil {
				return fmt.Errorf("finalizing PSBT funding "+
					"flow failed: %v", err)
			}

		case *lnrpc.OpenStatusUpdate_ChanPending:
			// As soon as the channel is pending, there is no more
			// shim that needs to be canceled. If the user
			// interrupts now, we don't need to clean up anything.
			shimPending = false

			err := printChanPending(update)
			if err != nil {
				return err
			}

			if !ctx.Bool("block") {
				return nil
			}

		case *lnrpc.OpenStatusUpdate_ChanOpen:
			return printChanOpen(update)
		}
	}
}

var batchOpenChannelCommand = cli.Command{
	Name:     "batchopenchannel",
	Category: "Channels",
	Usage: "Open multiple channels to existing peers in a single " +
		"transaction.",
	Description: `
	Attempt to open one or more new channels to an existing peer with the
	given node-keys.

	Example:
	lncli batchopenchannel --sat_per_vbyte=5 '[{
		"node_pubkey": "02abcdef...",
		"local_funding_amount": 500000,
		"private": true,
		"close_address": "bc1qxxx..."
	}, {
		"node_pubkey": "03fedcba...",
		"local_funding_amount": 200000,
		"remote_csv_delay": 288
	}]'

	All nodes listed must already be connected peers, otherwise funding will
	fail.

	The channel will be initialized with local_funding_amount satoshis 
	locally and push_sat satoshis for the remote node. Note that specifying 
	push_sat means you give that amount to the remote node as part of the 
	channel	opening. Once the channel is open, a channelPoint (txid:vout) of 
	the funding output is returned.

	If the remote peer supports the option upfront shutdown feature bit
	(query listpeers to see their supported feature bits), an address to
	enforce	payout of funds on cooperative close can optionally be provided.
	Note that if you set this value, you will not be able to cooperatively
	close out to another address.

	One can manually set the fee to be used for the funding transaction via
	either the --conf_target or --sat_per_vbyte arguments. This is optional.
`,
	ArgsUsage: "channels-json",
	Flags: []cli.Flag{
		cli.Int64Flag{
			Name: "conf_target",
			Usage: "(optional) the number of blocks that the " +
				"transaction *should* confirm in, will be " +
				"used for fee estimation",
		},
		cli.Int64Flag{
			Name: "sat_per_vbyte",
			Usage: "(optional) a manual fee expressed in " +
				"sat/vByte that should be used when crafting " +
				"the transaction",
		},
		cli.Uint64Flag{
			Name: "min_confs",
			Usage: "(optional) the minimum number of " +
				"confirmations each one of your outputs used " +
				"for the funding transaction must satisfy",
			Value: defaultUtxoMinConf,
		},
		cli.StringFlag{
			Name: "label",
			Usage: "(optional) a label to attach to the batch " +
				"transaction when storing it to the local " +
				"wallet after publishing it",
		},
	},
	Action: actionDecorator(batchOpenChannel),
}

type batchChannelJSON struct {
	NodePubkey         string `json:"node_pubkey,omitempty"`
	LocalFundingAmount int64  `json:"local_funding_amount,omitempty"`
	PushSat            int64  `json:"push_sat,omitempty"`
	Private            bool   `json:"private,omitempty"`
	MinHtlcMsat        int64  `json:"min_htlc_msat,omitempty"`
	RemoteCsvDelay     uint32 `json:"remote_csv_delay,omitempty"`
	CloseAddress       string `json:"close_address,omitempty"`
	PendingChanID      string `json:"pending_chan_id,omitempty"`
}

func batchOpenChannel(ctx *cli.Context) error {
	ctxc := getContext()
	client, cleanUp := getClient(ctx)
	defer cleanUp()

	args := ctx.Args()

	// Show command help if no arguments provided
	if ctx.NArg() == 0 {
		_ = cli.ShowCommandHelp(ctx, "batchopenchannel")
		return nil
	}

	minConfs := int32(ctx.Uint64("min_confs"))
	req := &lnrpc.BatchOpenChannelRequest{
		TargetConf:       int32(ctx.Int64("conf_target")),
		SatPerVbyte:      int64(ctx.Uint64("sat_per_vbyte")),
		MinConfs:         minConfs,
		SpendUnconfirmed: minConfs == 0,
		Label:            ctx.String("label"),
	}

	// Let's try and parse the JSON part of the CLI now. Fortunately we can
	// parse it directly into the RPC struct if we use the correct
	// marshaler that keeps the original snake case.
	var jsonChannels []*batchChannelJSON
	if err := json.Unmarshal([]byte(args.First()), &jsonChannels); err != nil {
		return fmt.Errorf("error parsing channels JSON: %v", err)
	}

	req.Channels = make([]*lnrpc.BatchOpenChannel, len(jsonChannels))
	for idx, jsonChannel := range jsonChannels {
		pubKeyBytes, err := hex.DecodeString(jsonChannel.NodePubkey)
		if err != nil {
			return fmt.Errorf("error parsing node pubkey hex: %v",
				err)
		}
		pendingChanBytes, err := hex.DecodeString(
			jsonChannel.PendingChanID,
		)
		if err != nil {
			return fmt.Errorf("error parsing pending chan ID: %v",
				err)
		}

		req.Channels[idx] = &lnrpc.BatchOpenChannel{
			NodePubkey:         pubKeyBytes,
			LocalFundingAmount: jsonChannel.LocalFundingAmount,
			PushSat:            jsonChannel.PushSat,
			Private:            jsonChannel.Private,
			MinHtlcMsat:        jsonChannel.MinHtlcMsat,
			RemoteCsvDelay:     jsonChannel.RemoteCsvDelay,
			CloseAddress:       jsonChannel.CloseAddress,
			PendingChanId:      pendingChanBytes,
		}
	}

	resp, err := client.BatchOpenChannel(ctxc, req)
	if err != nil {
		return err
	}

	for _, pending := range resp.PendingChannels {
		txid, err := chainhash.NewHash(pending.Txid)
		if err != nil {
			return err
		}

		printJSON(struct {
			FundingTxid        string `json:"funding_txid"`
			FundingOutputIndex uint32 `json:"funding_output_index"`
		}{
			FundingTxid:        txid.String(),
			FundingOutputIndex: pending.OutputIndex,
		})
	}

	return nil
}

// printChanOpen prints the channel point of the channel open message.
func printChanOpen(update *lnrpc.OpenStatusUpdate_ChanOpen) error {
	channelPoint := update.ChanOpen.ChannelPoint

	// A channel point's funding txid can be get/set as a
	// byte slice or a string. In the case it is a string,
	// decode it.
	var txidHash []byte
	switch channelPoint.GetFundingTxid().(type) {
	case *lnrpc.ChannelPoint_FundingTxidBytes:
		txidHash = channelPoint.GetFundingTxidBytes()
	case *lnrpc.ChannelPoint_FundingTxidStr:
		s := channelPoint.GetFundingTxidStr()
		h, err := chainhash.NewHashFromStr(s)
		if err != nil {
			return err
		}

		txidHash = h[:]
	}

	txid, err := chainhash.NewHash(txidHash)
	if err != nil {
		return err
	}

	index := channelPoint.OutputIndex
	printJSON(struct {
		ChannelPoint string `json:"channel_point"`
	}{
		ChannelPoint: fmt.Sprintf("%v:%v", txid, index),
	})
	return nil
}

// printChanPending prints the funding transaction ID of the channel pending
// message.
func printChanPending(update *lnrpc.OpenStatusUpdate_ChanPending) error {
	txid, err := chainhash.NewHash(update.ChanPending.Txid)
	if err != nil {
		return err
	}

	printJSON(struct {
		FundingTxid string `json:"funding_txid"`
	}{
		FundingTxid: txid.String(),
	})
	return nil
}

// readTerminalOrFile reads a single line from the terminal. If the line read is
// short enough to be a file and a file with that exact name exists, the content
// of that file is read and returned as a string. If the content is longer or no
// file exists, the string read from the terminal is returned directly. This
// function can be used to circumvent the N_TTY_BUF_SIZE kernel parameter that
// prevents pasting more than 4096 characters (on most systems) into a terminal.
func readTerminalOrFile(quit chan struct{}) (string, error) {
	maybeFile, err := readLine(quit)
	if err != nil {
		return "", err
	}

	// Absolute file paths normally can't be longer than 255 characters so
	// we don't even check if it's a file in that case.
	if len(maybeFile) > 255 {
		return maybeFile, nil
	}

	// It might be a file since the length is small enough. Calling os.Stat
	// should be safe with any arbitrary input as it will only query info
	// about the file, not open or execute it directly.
	stat, err := os.Stat(maybeFile)

	// The file doesn't exist, we must assume this wasn't a file path after
	// all.
	if err != nil && os.IsNotExist(err) {
		return maybeFile, nil
	}

	// Some other error, perhaps access denied or something similar, let's
	// surface that to the user.
	if err != nil {
		return "", err
	}

	// Make sure we don't read a huge file by accident which might lead to
	// undesired side effects. Even very large PSBTs should still only be a
	// few hundred kilobytes so it makes sense to put a cap here.
	if stat.Size() > psbtMaxFileSize {
		return "", fmt.Errorf("error reading file %s: size of %d "+
			"bytes exceeds max PSBT file size of %d", maybeFile,
			stat.Size(), psbtMaxFileSize)
	}

	// If it's a path to an existing file and it's small enough, let's try
	// to read its content now.
	content, err := ioutil.ReadFile(maybeFile)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// readLine reads a line from standard in but does not block in case of a
// system interrupt like syscall.SIGINT (Ctrl+C).
func readLine(quit chan struct{}) (string, error) {
	msg := make(chan string, 1)

	// In a normal console, reading from stdin won't signal EOF when the
	// user presses Ctrl+C. That's why we need to put this in a separate
	// goroutine so it doesn't block.
	go func() {
		for {
			var str string
			_, _ = fmt.Scan(&str)
			msg <- str
			return
		}
	}()
	for {
		select {
		case <-quit:
			return "", io.EOF

		case str := <-msg:
			return str, nil
		}
	}
}

// checkPsbtFlags make sure a request to open a channel doesn't set any
// parameters that are incompatible with the PSBT funding flow.
func checkPsbtFlags(req *lnrpc.OpenChannelRequest) error {
	if req.MinConfs != defaultUtxoMinConf || req.SpendUnconfirmed {
		return fmt.Errorf("specifying minimum confirmations for PSBT " +
			"funding is not supported")
	}
	if req.TargetConf != 0 || req.SatPerByte != 0 || req.SatPerVbyte != 0 { // nolint:staticcheck
		return fmt.Errorf("setting fee estimation parameters not " +
			"supported for PSBT funding")
	}
	return nil
}

// sendFundingState sends a single funding state step message by using a new
// client connection. This is necessary if the whole funding flow takes longer
// than the default macaroon timeout, then we cannot use a single client
// connection.
func sendFundingState(cancelCtx context.Context, cliCtx *cli.Context,
	msg *lnrpc.FundingTransitionMsg) error {

	client, cleanUp := getClient(cliCtx)
	defer cleanUp()

	_, err := client.FundingStateStep(cancelCtx, msg)
	return err
}

// finalizeMsgFromString creates the final message for the PsbtFinalize step
// from either a hex encoded raw wire transaction or a base64/binary encoded
// PSBT packet.
func finalizeMsgFromString(tx string,
	pendingChanID []byte) (*lnrpc.FundingTransitionMsg_PsbtFinalize,
	error) {

	psbtBytes, err := decodePsbt(tx)
	if err == nil {
		return &lnrpc.FundingTransitionMsg_PsbtFinalize{
			PsbtFinalize: &lnrpc.FundingPsbtFinalize{
				SignedPsbt:    psbtBytes,
				PendingChanId: pendingChanID,
			},
		}, nil
	}

	// PSBT decode failed, try to parse it as a hex encoded Bitcoin
	// transaction
	rawTx, err := hex.DecodeString(strings.TrimSpace(tx))
	if err != nil {
		return nil, fmt.Errorf("hex decode failed: %w", err)
	}
	msgtx := &wire.MsgTx{}
	err = msgtx.Deserialize(bytes.NewReader(rawTx))
	if err != nil {
		return nil, fmt.Errorf("deserializing as raw wire "+
			"transaction failed: %v", err)
	}
	return &lnrpc.FundingTransitionMsg_PsbtFinalize{
		PsbtFinalize: &lnrpc.FundingPsbtFinalize{
			FinalRawTx:    rawTx,
			PendingChanId: pendingChanID,
		},
	}, nil
}

// decodePsbt tries to decode the input as a binary or base64 PSBT. If this
// succeeded, the PSBT bytes are returned, an error otherwise.
func decodePsbt(psbt string) ([]byte, error) {
	switch {
	case strings.HasPrefix(psbt, "psbt\xff"):
		// A binary PSBT (read from a file) always starts with the PSBT
		// magic "psbt\xff" according to BIP 174
		return []byte(psbt), nil

	case strings.HasPrefix(strings.TrimSpace(psbt), "cHNidP"):
		// A base64 PSBT always starts with "cHNidP". This is the
		// longest base64 representation of the PSBT magic that is not
		// dependent on the byte after it.
		psbtBytes, err := base64.StdEncoding.DecodeString(
			strings.TrimSpace(psbt),
		)
		if err != nil {
			return nil, fmt.Errorf("base64 decode failed: %w", err)
		}

		return psbtBytes, nil

	default:
		return nil, fmt.Errorf("not a PSBT")
	}
}
