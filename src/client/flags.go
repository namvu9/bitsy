package client

// VERSION denotes the current version of the client and
// follows the <Major>.<Minor>.<Patch> Semantic Versioning
// scheme.
const VERSION = "1.0.0"

// CLIENTNAME is the human-readable identifier sent to
// other peers. It may, for example, be used in the
// handshake message of the Extension protocol
const CLIENTNAME = "Bitsy " + VERSION

// EXTPROT signals to other peers that the client
// implements the extension negotiation protocl defined in
// BEP-10
const EXTPROT = 0x10

// Sets the reserved bits
var bitmask = [8]byte{
	0x0,
	0x0,
	0x0,
	0x0,
	0x0,
	0x0 | EXTPROT,
	0x0,
	0x0,
}
