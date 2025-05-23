package lnwire

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image/color"
	"io"
	"math"
	"math/rand"
	"net"
	"reflect"
	"testing"
	"testing/quick"
	"time"

	"github.com/ltcsuite/lnd/tor"
	"github.com/ltcsuite/ltcd/btcec/v2"
	"github.com/ltcsuite/ltcd/btcec/v2/ecdsa"
	"github.com/ltcsuite/ltcd/chaincfg/chainhash"
	"github.com/ltcsuite/ltcd/ltcutil"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	shaHash1Bytes, _ = hex.DecodeString("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
	shaHash1, _      = chainhash.NewHash(shaHash1Bytes)
	outpoint1        = wire.NewOutPoint(shaHash1, 0)

	testRBytes, _ = hex.DecodeString("8ce2bc69281ce27da07e6683571319d18e949ddfa2965fb6caa1bf0314f882d7")
	testSBytes, _ = hex.DecodeString("299105481d63e0f4bc2a88121167221b6700d72a0ead154c03be696a292d24ae")
	testRScalar   = new(btcec.ModNScalar)
	testSScalar   = new(btcec.ModNScalar)
	_             = testRScalar.SetByteSlice(testRBytes)
	_             = testSScalar.SetByteSlice(testSBytes)
	testSig       = ecdsa.NewSignature(testRScalar, testSScalar)
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randLocalNonce(r *rand.Rand) *Musig2Nonce {
	var nonce Musig2Nonce
	_, _ = io.ReadFull(r, nonce[:])

	return &nonce
}

func randPartialSig(r *rand.Rand) (*PartialSig, error) {
	var sigBytes [32]byte
	if _, err := r.Read(sigBytes[:]); err != nil {
		return nil, fmt.Errorf("unable to generate sig: %w", err)
	}

	var s btcec.ModNScalar
	s.SetByteSlice(sigBytes[:])

	return &PartialSig{
		Sig: s,
	}, nil
}

func randPartialSigWithNonce(r *rand.Rand) (*PartialSigWithNonce, error) {
	var sigBytes [32]byte
	if _, err := r.Read(sigBytes[:]); err != nil {
		return nil, fmt.Errorf("unable to generate sig: %w", err)
	}

	var s btcec.ModNScalar
	s.SetByteSlice(sigBytes[:])

	return &PartialSigWithNonce{
		PartialSig: NewPartialSig(s),
		Nonce:      *randLocalNonce(r),
	}, nil
}

func randAlias(r *rand.Rand) NodeAlias {
	var a NodeAlias
	for i := range a {
		a[i] = letterBytes[r.Intn(len(letterBytes))]
	}

	return a
}

func randPubKey() (*btcec.PublicKey, error) {
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, err
	}

	return priv.PubKey(), nil
}

func randRawKey() ([33]byte, error) {
	var n [33]byte

	priv, err := btcec.NewPrivateKey()
	if err != nil {
		return n, err
	}

	copy(n[:], priv.PubKey().SerializeCompressed())

	return n, nil
}

func randDeliveryAddress(r *rand.Rand) (DeliveryAddress, error) {
	// Generate size minimum one. Empty scripts should be tested specifically.
	size := r.Intn(deliveryAddressMaxSize) + 1
	da := DeliveryAddress(make([]byte, size))

	_, err := r.Read(da)
	return da, err
}

func randRawFeatureVector(r *rand.Rand) *RawFeatureVector {
	featureVec := NewRawFeatureVector()
	for i := 0; i < 10000; i++ {
		if r.Int31n(2) == 0 {
			featureVec.Set(FeatureBit(i))
		}
	}
	return featureVec
}

func randTCP4Addr(r *rand.Rand) (*net.TCPAddr, error) {
	var ip [4]byte
	if _, err := r.Read(ip[:]); err != nil {
		return nil, err
	}

	var port [2]byte
	if _, err := r.Read(port[:]); err != nil {
		return nil, err
	}

	addrIP := net.IP(ip[:])
	addrPort := int(binary.BigEndian.Uint16(port[:]))

	return &net.TCPAddr{IP: addrIP, Port: addrPort}, nil
}

func randTCP6Addr(r *rand.Rand) (*net.TCPAddr, error) {
	var ip [16]byte
	if _, err := r.Read(ip[:]); err != nil {
		return nil, err
	}

	var port [2]byte
	if _, err := r.Read(port[:]); err != nil {
		return nil, err
	}

	addrIP := net.IP(ip[:])
	addrPort := int(binary.BigEndian.Uint16(port[:]))

	return &net.TCPAddr{IP: addrIP, Port: addrPort}, nil
}

func randV2OnionAddr(r *rand.Rand) (*tor.OnionAddr, error) {
	var serviceID [tor.V2DecodedLen]byte
	if _, err := r.Read(serviceID[:]); err != nil {
		return nil, err
	}

	var port [2]byte
	if _, err := r.Read(port[:]); err != nil {
		return nil, err
	}

	onionService := tor.Base32Encoding.EncodeToString(serviceID[:])
	onionService += tor.OnionSuffix
	addrPort := int(binary.BigEndian.Uint16(port[:]))

	return &tor.OnionAddr{OnionService: onionService, Port: addrPort}, nil
}

func randV3OnionAddr(r *rand.Rand) (*tor.OnionAddr, error) {
	var serviceID [tor.V3DecodedLen]byte
	if _, err := r.Read(serviceID[:]); err != nil {
		return nil, err
	}

	var port [2]byte
	if _, err := r.Read(port[:]); err != nil {
		return nil, err
	}

	onionService := tor.Base32Encoding.EncodeToString(serviceID[:])
	onionService += tor.OnionSuffix
	addrPort := int(binary.BigEndian.Uint16(port[:]))

	return &tor.OnionAddr{OnionService: onionService, Port: addrPort}, nil
}

func randOpaqueAddr(r *rand.Rand) (*OpaqueAddrs, error) {
	payloadLen := r.Int63n(64) + 1
	payload := make([]byte, payloadLen)

	// The first byte is the address type. So set it to one that we
	// definitely don't know about.
	payload[0] = math.MaxUint8

	// Generate random bytes for the rest of the payload.
	if _, err := r.Read(payload[1:]); err != nil {
		return nil, err
	}

	return &OpaqueAddrs{Payload: payload}, nil
}

func randAddrs(r *rand.Rand) ([]net.Addr, error) {
	tcp4Addr, err := randTCP4Addr(r)
	if err != nil {
		return nil, err
	}

	tcp6Addr, err := randTCP6Addr(r)
	if err != nil {
		return nil, err
	}

	v2OnionAddr, err := randV2OnionAddr(r)
	if err != nil {
		return nil, err
	}

	v3OnionAddr, err := randV3OnionAddr(r)
	if err != nil {
		return nil, err
	}

	opaqueAddrs, err := randOpaqueAddr(r)
	if err != nil {
		return nil, err
	}

	return []net.Addr{
		tcp4Addr, tcp6Addr, v2OnionAddr, v3OnionAddr, opaqueAddrs,
	}, nil
}

// TestChanUpdateChanFlags ensures that converting the ChanUpdateChanFlags and
// ChanUpdateMsgFlags bitfields to a string behaves as expected.
func TestChanUpdateChanFlags(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		flags    uint8
		expected string
	}{
		{
			flags:    0,
			expected: "00000000",
		},
		{
			flags:    1,
			expected: "00000001",
		},
		{
			flags:    3,
			expected: "00000011",
		},
		{
			flags:    255,
			expected: "11111111",
		},
	}

	for _, test := range testCases {
		chanFlag := ChanUpdateChanFlags(test.flags)
		toStr := chanFlag.String()
		if toStr != test.expected {
			t.Fatalf("expected %v, got %v",
				test.expected, toStr)
		}

		msgFlag := ChanUpdateMsgFlags(test.flags)
		toStr = msgFlag.String()
		if toStr != test.expected {
			t.Fatalf("expected %v, got %v",
				test.expected, toStr)
		}
	}
}

// TestDecodeUnknownAddressType shows that an unknown address type is currently
// incorrectly dealt with.
func TestDecodeUnknownAddressType(t *testing.T) {
	// Add a normal, clearnet address.
	tcpAddr := &net.TCPAddr{
		IP:   net.IP{127, 0, 0, 1},
		Port: 8080,
	}

	// Add an onion address.
	onionAddr := &tor.OnionAddr{
		OnionService: "abcdefghijklmnop.onion",
		Port:         9065,
	}

	// Now add an address with an unknown type.
	var newAddrType addressType = math.MaxUint8
	data := make([]byte, 0, 16)
	data = append(data, uint8(newAddrType))
	opaqueAddrs := &OpaqueAddrs{
		Payload: data,
	}

	buffer := bytes.NewBuffer(make([]byte, 0, MaxMsgBody))
	err := WriteNetAddrs(
		buffer, []net.Addr{tcpAddr, onionAddr, opaqueAddrs},
	)
	require.NoError(t, err)

	// Now we attempt to parse the bytes and assert that we get an error.
	var addrs []net.Addr
	err = ReadElement(buffer, &addrs)
	require.NoError(t, err)
	require.Len(t, addrs, 3)
	require.Equal(t, tcpAddr.String(), addrs[0].String())
	require.Equal(t, onionAddr.String(), addrs[1].String())
	require.Equal(t, hex.EncodeToString(data), addrs[2].String())
}

func TestMaxOutPointIndex(t *testing.T) {
	t.Parallel()

	op := wire.OutPoint{
		Index: math.MaxUint32,
	}

	var b bytes.Buffer
	if err := WriteOutPoint(&b, op); err == nil {
		t.Fatalf("write of outPoint should fail, index exceeds 16-bits")
	}
}

func TestEmptyMessageUnknownType(t *testing.T) {
	t.Parallel()

	fakeType := CustomTypeStart - 1
	if _, err := makeEmptyMessage(fakeType); err == nil {
		t.Fatalf("should not be able to make an empty message of an " +
			"unknown type")
	}
}

// TestLightningWireProtocol uses the testing/quick package to create a series
// of fuzz tests to attempt to break a primary scenario which is implemented as
// property based testing scenario.
func TestLightningWireProtocol(t *testing.T) {
	t.Parallel()

	// mainScenario is the primary test that will programmatically be
	// executed for all registered wire messages. The quick-checker within
	// testing/quick will attempt to find an input to this function, s.t
	// the function returns false, if so then we've found an input that
	// violates our model of the system.
	mainScenario := func(msg Message) bool {
		// Give a new message, we'll serialize the message into a new
		// bytes buffer.
		var b bytes.Buffer
		if _, err := WriteMessage(&b, msg, 0); err != nil {
			t.Fatalf("unable to write msg: %v", err)
			return false
		}

		// Next, we'll ensure that the serialized payload (subtracting
		// the 2 bytes for the message type) is _below_ the specified
		// max payload size for this message.
		payloadLen := uint32(b.Len()) - 2
		if payloadLen > MaxMsgBody {
			t.Fatalf("msg payload constraint violated: %v > %v",
				payloadLen, MaxMsgBody)
			return false
		}

		// Finally, we'll deserialize the message from the written
		// buffer, and finally assert that the messages are equal.
		newMsg, err := ReadMessage(&b, 0)
		if err != nil {
			t.Fatalf("unable to read msg: %v", err)
			return false
		}
		if !assert.Equalf(t, msg, newMsg, "message mismatch") {
			return false
		}

		return true
	}

	// customTypeGen is a map of functions that are able to randomly
	// generate a given type. These functions are needed for types which
	// are too complex for the testing/quick package to automatically
	// generate.
	customTypeGen := map[MessageType]func([]reflect.Value, *rand.Rand){
		MsgInit: func(v []reflect.Value, r *rand.Rand) {
			req := NewInitMessage(
				randRawFeatureVector(r),
				randRawFeatureVector(r),
			)

			v[0] = reflect.ValueOf(*req)
		},
		MsgOpenChannel: func(v []reflect.Value, r *rand.Rand) {
			req := OpenChannel{
				FundingAmount:    ltcutil.Amount(r.Int63()),
				PushAmount:       MilliSatoshi(r.Int63()),
				DustLimit:        ltcutil.Amount(r.Int63()),
				MaxValueInFlight: MilliSatoshi(r.Int63()),
				ChannelReserve:   ltcutil.Amount(r.Int63()),
				HtlcMinimum:      MilliSatoshi(r.Int31()),
				FeePerKiloWeight: uint32(r.Int63()),
				CsvDelay:         uint16(r.Int31()),
				MaxAcceptedHTLCs: uint16(r.Int31()),
				ChannelFlags:     FundingFlag(uint8(r.Int31())),
			}

			if _, err := r.Read(req.ChainHash[:]); err != nil {
				t.Fatalf("unable to generate chain hash: %v", err)
				return
			}

			if _, err := r.Read(req.PendingChannelID[:]); err != nil {
				t.Fatalf("unable to generate pending chan id: %v", err)
				return
			}

			var err error
			req.FundingKey, err = randPubKey()
			if err != nil {
				t.Fatalf("unable to generate key: %v", err)
				return
			}
			req.RevocationPoint, err = randPubKey()
			if err != nil {
				t.Fatalf("unable to generate key: %v", err)
				return
			}
			req.PaymentPoint, err = randPubKey()
			if err != nil {
				t.Fatalf("unable to generate key: %v", err)
				return
			}
			req.DelayedPaymentPoint, err = randPubKey()
			if err != nil {
				t.Fatalf("unable to generate key: %v", err)
				return
			}
			req.HtlcPoint, err = randPubKey()
			if err != nil {
				t.Fatalf("unable to generate key: %v", err)
				return
			}
			req.FirstCommitmentPoint, err = randPubKey()
			if err != nil {
				t.Fatalf("unable to generate key: %v", err)
				return
			}

			// 1/2 chance empty TLV records.
			if r.Intn(2) == 0 {
				req.UpfrontShutdownScript, err = randDeliveryAddress(r)
				if err != nil {
					t.Fatalf("unable to generate delivery address: %v", err)
					return
				}

				req.ChannelType = new(ChannelType)
				*req.ChannelType = ChannelType(*randRawFeatureVector(r))

				req.LeaseExpiry = new(LeaseExpiry)
				*req.LeaseExpiry = LeaseExpiry(1337)

				req.LocalNonce = randLocalNonce(r)
			} else {
				req.UpfrontShutdownScript = []byte{}
			}

			// 1/2 chance additional TLV data.
			if r.Intn(2) == 0 {
				req.ExtraData = []byte{0xfd, 0x00, 0xff, 0x00}
			}

			v[0] = reflect.ValueOf(req)
		},
		MsgAcceptChannel: func(v []reflect.Value, r *rand.Rand) {
			req := AcceptChannel{
				DustLimit:        ltcutil.Amount(r.Int63()),
				MaxValueInFlight: MilliSatoshi(r.Int63()),
				ChannelReserve:   ltcutil.Amount(r.Int63()),
				MinAcceptDepth:   uint32(r.Int31()),
				HtlcMinimum:      MilliSatoshi(r.Int31()),
				CsvDelay:         uint16(r.Int31()),
				MaxAcceptedHTLCs: uint16(r.Int31()),
			}

			if _, err := r.Read(req.PendingChannelID[:]); err != nil {
				t.Fatalf("unable to generate pending chan id: %v", err)
				return
			}

			var err error
			req.FundingKey, err = randPubKey()
			if err != nil {
				t.Fatalf("unable to generate key: %v", err)
				return
			}
			req.RevocationPoint, err = randPubKey()
			if err != nil {
				t.Fatalf("unable to generate key: %v", err)
				return
			}
			req.PaymentPoint, err = randPubKey()
			if err != nil {
				t.Fatalf("unable to generate key: %v", err)
				return
			}
			req.DelayedPaymentPoint, err = randPubKey()
			if err != nil {
				t.Fatalf("unable to generate key: %v", err)
				return
			}
			req.HtlcPoint, err = randPubKey()
			if err != nil {
				t.Fatalf("unable to generate key: %v", err)
				return
			}
			req.FirstCommitmentPoint, err = randPubKey()
			if err != nil {
				t.Fatalf("unable to generate key: %v", err)
				return
			}

			// 1/2 chance empty TLV records.
			if r.Intn(2) == 0 {
				req.UpfrontShutdownScript, err = randDeliveryAddress(r)
				if err != nil {
					t.Fatalf("unable to generate delivery address: %v", err)
					return
				}

				req.ChannelType = new(ChannelType)
				*req.ChannelType = ChannelType(*randRawFeatureVector(r))

				req.LeaseExpiry = new(LeaseExpiry)
				*req.LeaseExpiry = LeaseExpiry(1337)

				req.LocalNonce = randLocalNonce(r)
			} else {
				req.UpfrontShutdownScript = []byte{}
			}

			// 1/2 chance additional TLV data.
			if r.Intn(2) == 0 {
				req.ExtraData = []byte{0xfd, 0x00, 0xff, 0x00}
			}

			v[0] = reflect.ValueOf(req)
		},
		MsgFundingCreated: func(v []reflect.Value, r *rand.Rand) {
			req := FundingCreated{
				ExtraData: make([]byte, 0),
			}

			if _, err := r.Read(req.PendingChannelID[:]); err != nil {
				t.Fatalf("unable to generate pending chan id: %v", err)
				return
			}

			if _, err := r.Read(req.FundingPoint.Hash[:]); err != nil {
				t.Fatalf("unable to generate hash: %v", err)
				return
			}
			req.FundingPoint.Index = uint32(r.Int31()) % math.MaxUint16

			var err error
			req.CommitSig, err = NewSigFromSignature(testSig)
			if err != nil {
				t.Fatalf("unable to parse sig: %v", err)
				return
			}

			// 1/2 chance to attach a partial sig.
			if r.Intn(2) == 0 {
				req.PartialSig, err = randPartialSigWithNonce(r)
				if err != nil {
					t.Fatalf("unable to generate sig: %v",
						err)
					return
				}
			}

			v[0] = reflect.ValueOf(req)
		},
		MsgFundingSigned: func(v []reflect.Value, r *rand.Rand) {
			var c [32]byte
			_, err := r.Read(c[:])
			if err != nil {
				t.Fatalf("unable to generate chan id: %v", err)
				return
			}

			req := FundingSigned{
				ChanID:    ChannelID(c),
				ExtraData: make([]byte, 0),
			}
			req.CommitSig, err = NewSigFromSignature(testSig)
			if err != nil {
				t.Fatalf("unable to parse sig: %v", err)
				return
			}

			// 1/2 chance to attach a partial sig.
			if r.Intn(2) == 0 {
				req.PartialSig, err = randPartialSigWithNonce(r)
				if err != nil {
					t.Fatalf("unable to generate sig: %v",
						err)
					return
				}
			}

			v[0] = reflect.ValueOf(req)
		},
		MsgChannelReady: func(v []reflect.Value, r *rand.Rand) {
			var c [32]byte
			if _, err := r.Read(c[:]); err != nil {
				t.Fatalf("unable to generate chan id: %v", err)
				return
			}

			pubKey, err := randPubKey()
			if err != nil {
				t.Fatalf("unable to generate key: %v", err)
				return
			}

			req := NewChannelReady(ChannelID(c), pubKey)

			if r.Int31()%2 == 0 {
				scid := NewShortChanIDFromInt(uint64(r.Int63()))
				req.AliasScid = &scid
				req.NextLocalNonce = randLocalNonce(r)
			}

			v[0] = reflect.ValueOf(*req)
		},
		MsgShutdown: func(v []reflect.Value, r *rand.Rand) {
			var c [32]byte
			_, err := r.Read(c[:])
			if err != nil {
				t.Fatalf("unable to generate chan id: %v", err)
				return
			}

			shutdownAddr, err := randDeliveryAddress(r)
			if err != nil {
				t.Fatalf("unable to generate delivery "+
					"address: %v", err)
				return
			}

			req := Shutdown{
				ChannelID: ChannelID(c),
				Address:   shutdownAddr,
				ExtraData: make([]byte, 0),
			}

			if r.Int31()%2 == 0 {
				req.ShutdownNonce = (*ShutdownNonce)(
					randLocalNonce(r),
				)
			}

			v[0] = reflect.ValueOf(req)
		},
		MsgClosingSigned: func(v []reflect.Value, r *rand.Rand) {
			req := ClosingSigned{
				FeeSatoshis: ltcutil.Amount(r.Int63()),
				ExtraData:   make([]byte, 0),
			}
			var err error
			req.Signature, err = NewSigFromSignature(testSig)
			if err != nil {
				t.Fatalf("unable to parse sig: %v", err)
				return
			}

			if _, err := r.Read(req.ChannelID[:]); err != nil {
				t.Fatalf("unable to generate chan id: %v", err)
				return
			}

			if r.Int31()%2 == 0 {
				req.PartialSig, err = randPartialSig(r)
				if err != nil {
					t.Fatalf("unable to generate sig: %v",
						err)
					return
				}
			}

			v[0] = reflect.ValueOf(req)
		},
		MsgCommitSig: func(v []reflect.Value, r *rand.Rand) {
			req := NewCommitSig()
			if _, err := r.Read(req.ChanID[:]); err != nil {
				t.Fatalf("unable to generate chan id: %v", err)
				return
			}

			var err error
			req.CommitSig, err = NewSigFromSignature(testSig)
			if err != nil {
				t.Fatalf("unable to parse sig: %v", err)
				return
			}

			// Only create the slice if there will be any signatures
			// in it to prevent false positive test failures due to
			// an empty slice versus a nil slice.
			numSigs := uint16(r.Int31n(1019))
			if numSigs > 0 {
				req.HtlcSigs = make([]Sig, numSigs)
			}
			for i := 0; i < int(numSigs); i++ {
				req.HtlcSigs[i], err = NewSigFromSignature(testSig)
				if err != nil {
					t.Fatalf("unable to parse sig: %v", err)
					return
				}
			}

			// 50/50 chance to attach a partial sig.
			if r.Int31()%2 == 0 {
				req.PartialSig, err = randPartialSigWithNonce(r)
				if err != nil {
					t.Fatalf("unable to generate sig: %v",
						err)
					return
				}
			}

			v[0] = reflect.ValueOf(*req)
		},
		MsgRevokeAndAck: func(v []reflect.Value, r *rand.Rand) {
			req := NewRevokeAndAck()
			if _, err := r.Read(req.ChanID[:]); err != nil {
				t.Fatalf("unable to generate chan id: %v", err)
				return
			}
			if _, err := r.Read(req.Revocation[:]); err != nil {
				t.Fatalf("unable to generate bytes: %v", err)
				return
			}
			var err error
			req.NextRevocationKey, err = randPubKey()
			if err != nil {
				t.Fatalf("unable to generate key: %v", err)
				return
			}

			// 50/50 chance to attach a local nonce.
			if r.Int31()%2 == 0 {
				req.LocalNonce = randLocalNonce(r)
			}

			v[0] = reflect.ValueOf(*req)
		},
		MsgChannelAnnouncement: func(v []reflect.Value, r *rand.Rand) {
			var err error
			req := ChannelAnnouncement{
				ShortChannelID:  NewShortChanIDFromInt(uint64(r.Int63())),
				Features:        randRawFeatureVector(r),
				ExtraOpaqueData: make([]byte, 0),
			}
			req.NodeSig1, err = NewSigFromSignature(testSig)
			if err != nil {
				t.Fatalf("unable to parse sig: %v", err)
				return
			}
			req.NodeSig2, err = NewSigFromSignature(testSig)
			if err != nil {
				t.Fatalf("unable to parse sig: %v", err)
				return
			}
			req.BitcoinSig1, err = NewSigFromSignature(testSig)
			if err != nil {
				t.Fatalf("unable to parse sig: %v", err)
				return
			}
			req.BitcoinSig2, err = NewSigFromSignature(testSig)
			if err != nil {
				t.Fatalf("unable to parse sig: %v", err)
				return
			}

			req.NodeID1, err = randRawKey()
			if err != nil {
				t.Fatalf("unable to generate key: %v", err)
				return
			}
			req.NodeID2, err = randRawKey()
			if err != nil {
				t.Fatalf("unable to generate key: %v", err)
				return
			}
			req.BitcoinKey1, err = randRawKey()
			if err != nil {
				t.Fatalf("unable to generate key: %v", err)
				return
			}
			req.BitcoinKey2, err = randRawKey()
			if err != nil {
				t.Fatalf("unable to generate key: %v", err)
				return
			}
			if _, err := r.Read(req.ChainHash[:]); err != nil {
				t.Fatalf("unable to generate chain hash: %v", err)
				return
			}

			numExtraBytes := r.Int31n(1000)
			if numExtraBytes > 0 {
				req.ExtraOpaqueData = make([]byte, numExtraBytes)
				_, err := r.Read(req.ExtraOpaqueData[:])
				if err != nil {
					t.Fatalf("unable to generate opaque "+
						"bytes: %v", err)
					return
				}
			}

			v[0] = reflect.ValueOf(req)
		},
		MsgNodeAnnouncement: func(v []reflect.Value, r *rand.Rand) {
			var err error
			req := NodeAnnouncement{
				Features:  randRawFeatureVector(r),
				Timestamp: uint32(r.Int31()),
				Alias:     randAlias(r),
				RGBColor: color.RGBA{
					R: uint8(r.Int31()),
					G: uint8(r.Int31()),
					B: uint8(r.Int31()),
				},
				ExtraOpaqueData: make([]byte, 0),
			}
			req.Signature, err = NewSigFromSignature(testSig)
			if err != nil {
				t.Fatalf("unable to parse sig: %v", err)
				return
			}

			req.NodeID, err = randRawKey()
			if err != nil {
				t.Fatalf("unable to generate key: %v", err)
				return
			}

			req.Addresses, err = randAddrs(r)
			if err != nil {
				t.Fatalf("unable to generate addresses: %v", err)
			}

			numExtraBytes := r.Int31n(1000)
			if numExtraBytes > 0 {
				req.ExtraOpaqueData = make([]byte, numExtraBytes)
				_, err := r.Read(req.ExtraOpaqueData[:])
				if err != nil {
					t.Fatalf("unable to generate opaque "+
						"bytes: %v", err)
					return
				}
			}

			v[0] = reflect.ValueOf(req)
		},
		MsgChannelUpdate: func(v []reflect.Value, r *rand.Rand) {
			var err error

			msgFlags := ChanUpdateMsgFlags(r.Int31())
			maxHtlc := MilliSatoshi(r.Int63())

			// We make the max_htlc field zero if it is not flagged
			// as being part of the ChannelUpdate, to pass
			// serialization tests, as it will be ignored if the bit
			// is not set.
			if msgFlags&ChanUpdateRequiredMaxHtlc == 0 {
				maxHtlc = 0
			}

			req := ChannelUpdate{
				ShortChannelID:  NewShortChanIDFromInt(uint64(r.Int63())),
				Timestamp:       uint32(r.Int31()),
				MessageFlags:    msgFlags,
				ChannelFlags:    ChanUpdateChanFlags(r.Int31()),
				TimeLockDelta:   uint16(r.Int31()),
				HtlcMinimumMsat: MilliSatoshi(r.Int63()),
				HtlcMaximumMsat: maxHtlc,
				BaseFee:         uint32(r.Int31()),
				FeeRate:         uint32(r.Int31()),
				ExtraOpaqueData: make([]byte, 0),
			}
			req.Signature, err = NewSigFromSignature(testSig)
			if err != nil {
				t.Fatalf("unable to parse sig: %v", err)
				return
			}

			if _, err := r.Read(req.ChainHash[:]); err != nil {
				t.Fatalf("unable to generate chain hash: %v", err)
				return
			}

			numExtraBytes := r.Int31n(1000)
			if numExtraBytes > 0 {
				req.ExtraOpaqueData = make([]byte, numExtraBytes)
				_, err := r.Read(req.ExtraOpaqueData[:])
				if err != nil {
					t.Fatalf("unable to generate opaque "+
						"bytes: %v", err)
					return
				}
			}

			v[0] = reflect.ValueOf(req)
		},
		MsgAnnounceSignatures: func(v []reflect.Value, r *rand.Rand) {
			var err error
			req := AnnounceSignatures{
				ShortChannelID:  NewShortChanIDFromInt(uint64(r.Int63())),
				ExtraOpaqueData: make([]byte, 0),
			}

			req.NodeSignature, err = NewSigFromSignature(testSig)
			if err != nil {
				t.Fatalf("unable to parse sig: %v", err)
				return
			}

			req.BitcoinSignature, err = NewSigFromSignature(testSig)
			if err != nil {
				t.Fatalf("unable to parse sig: %v", err)
				return
			}

			if _, err := r.Read(req.ChannelID[:]); err != nil {
				t.Fatalf("unable to generate chan id: %v", err)
				return
			}

			numExtraBytes := r.Int31n(1000)
			if numExtraBytes > 0 {
				req.ExtraOpaqueData = make([]byte, numExtraBytes)
				_, err := r.Read(req.ExtraOpaqueData[:])
				if err != nil {
					t.Fatalf("unable to generate opaque "+
						"bytes: %v", err)
					return
				}
			}

			v[0] = reflect.ValueOf(req)
		},
		MsgChannelReestablish: func(v []reflect.Value, r *rand.Rand) {
			req := ChannelReestablish{
				NextLocalCommitHeight:  uint64(r.Int63()),
				RemoteCommitTailHeight: uint64(r.Int63()),
				ExtraData:              make([]byte, 0),
			}

			// With a 50/50 probability, we'll include the
			// additional fields so we can test our ability to
			// properly parse, and write out the optional fields.
			if r.Int()%2 == 0 {
				_, err := r.Read(req.LastRemoteCommitSecret[:])
				if err != nil {
					t.Fatalf("unable to read commit secret: %v", err)
					return
				}

				req.LocalUnrevokedCommitPoint, err = randPubKey()
				if err != nil {
					t.Fatalf("unable to generate key: %v", err)
					return
				}

				req.LocalNonce = randLocalNonce(r)
			}

			v[0] = reflect.ValueOf(req)
		},
		MsgQueryShortChanIDs: func(v []reflect.Value, r *rand.Rand) {
			req := QueryShortChanIDs{
				ExtraData: make([]byte, 0),
			}

			// With a 50/50 change, we'll either use zlib encoding,
			// or regular encoding.
			if r.Int31()%2 == 0 {
				req.EncodingType = EncodingSortedZlib
			} else {
				req.EncodingType = EncodingSortedPlain
			}

			if _, err := rand.Read(req.ChainHash[:]); err != nil {
				t.Fatalf("unable to read chain hash: %v", err)
				return
			}

			numChanIDs := rand.Int31n(5000)
			for i := int32(0); i < numChanIDs; i++ {
				req.ShortChanIDs = append(req.ShortChanIDs,
					NewShortChanIDFromInt(uint64(r.Int63())))
			}

			v[0] = reflect.ValueOf(req)
		},
		MsgReplyChannelRange: func(v []reflect.Value, r *rand.Rand) {
			req := ReplyChannelRange{
				FirstBlockHeight: uint32(r.Int31()),
				NumBlocks:        uint32(r.Int31()),
				ExtraData:        make([]byte, 0),
			}

			if _, err := rand.Read(req.ChainHash[:]); err != nil {
				t.Fatalf("unable to read chain hash: %v", err)
				return
			}

			req.Complete = uint8(r.Int31n(2))

			// With a 50/50 change, we'll either use zlib encoding,
			// or regular encoding.
			if r.Int31()%2 == 0 {
				req.EncodingType = EncodingSortedZlib
			} else {
				req.EncodingType = EncodingSortedPlain
			}

			numChanIDs := rand.Int31n(5000)
			for i := int32(0); i < numChanIDs; i++ {
				req.ShortChanIDs = append(req.ShortChanIDs,
					NewShortChanIDFromInt(uint64(r.Int63())))
			}

			v[0] = reflect.ValueOf(req)
		},
		MsgPing: func(v []reflect.Value, r *rand.Rand) {
			// We use a special message generator here to ensure we
			// don't generate ping messages that are too large,
			// which'll cause the test to fail.
			//
			// We'll allow the test to generate padding bytes up to
			// the max message limit, factoring in the 2 bytes for
			// the num pong bytes and 2 bytes for encoding the
			// length of the padding bytes.
			paddingBytes := make([]byte, rand.Intn(MaxMsgBody-3))
			req := Ping{
				NumPongBytes: uint16(r.Intn(MaxPongBytes + 1)),
				PaddingBytes: paddingBytes,
			}

			v[0] = reflect.ValueOf(req)
		},
	}

	// With the above types defined, we'll now generate a slice of
	// scenarios to feed into quick.Check. The function scans in input
	// space of the target function under test, so we'll need to create a
	// series of wrapper functions to force it to iterate over the target
	// types, but re-use the mainScenario defined above.
	tests := []struct {
		msgType  MessageType
		scenario interface{}
	}{
		{
			msgType: MsgInit,
			scenario: func(m Init) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgWarning,
			scenario: func(m Warning) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgError,
			scenario: func(m Error) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgPing,
			scenario: func(m Ping) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgPong,
			scenario: func(m Pong) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgOpenChannel,
			scenario: func(m OpenChannel) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgAcceptChannel,
			scenario: func(m AcceptChannel) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgFundingCreated,
			scenario: func(m FundingCreated) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgFundingSigned,
			scenario: func(m FundingSigned) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgChannelReady,
			scenario: func(m ChannelReady) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgShutdown,
			scenario: func(m Shutdown) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgClosingSigned,
			scenario: func(m ClosingSigned) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgUpdateAddHTLC,
			scenario: func(m UpdateAddHTLC) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgUpdateFulfillHTLC,
			scenario: func(m UpdateFulfillHTLC) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgUpdateFailHTLC,
			scenario: func(m UpdateFailHTLC) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgCommitSig,
			scenario: func(m CommitSig) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgRevokeAndAck,
			scenario: func(m RevokeAndAck) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgUpdateFee,
			scenario: func(m UpdateFee) bool {
				return mainScenario(&m)
			},
		},
		{

			msgType: MsgUpdateFailMalformedHTLC,
			scenario: func(m UpdateFailMalformedHTLC) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgChannelReestablish,
			scenario: func(m ChannelReestablish) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgChannelAnnouncement,
			scenario: func(m ChannelAnnouncement) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgNodeAnnouncement,
			scenario: func(m NodeAnnouncement) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgChannelUpdate,
			scenario: func(m ChannelUpdate) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgAnnounceSignatures,
			scenario: func(m AnnounceSignatures) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgGossipTimestampRange,
			scenario: func(m GossipTimestampRange) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgQueryShortChanIDs,
			scenario: func(m QueryShortChanIDs) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgReplyShortChanIDsEnd,
			scenario: func(m ReplyShortChanIDsEnd) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgQueryChannelRange,
			scenario: func(m QueryChannelRange) bool {
				return mainScenario(&m)
			},
		},
		{
			msgType: MsgReplyChannelRange,
			scenario: func(m ReplyChannelRange) bool {
				return mainScenario(&m)
			},
		},
	}
	for _, test := range tests {
		var config *quick.Config

		// If the type defined is within the custom type gen map above,
		// then we'll modify the default config to use this Value
		// function that knows how to generate the proper types.
		if valueGen, ok := customTypeGen[test.msgType]; ok {
			config = &quick.Config{
				Values: valueGen,
			}
		}

		t.Logf("Running fuzz tests for msgType=%v", test.msgType)
		if err := quick.Check(test.scenario, config); err != nil {
			t.Fatalf("fuzz checks for msg=%v failed: %v",
				test.msgType, err)
		}
	}

}

func init() {
	rand.Seed(time.Now().Unix())
}
