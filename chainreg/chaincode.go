package chainreg

// ChainCode is an enum-like structure for keeping track of the chains
// currently supported within lnd.
type ChainCode uint32

const (
	// LitecoinChain is Litecoin's chain.
	LitecoinChain ChainCode = 1
)

// String returns a string representation of the target ChainCode.
func (c ChainCode) String() string {
	switch c {
	case LitecoinChain:
		return "litecoin"
	default:
		return "kekcoin"
	}
}
