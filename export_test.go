package macaroon

// Data returns the macaroon's data.
func (m *Macaroon) Data() []byte {
	return m.data
}

// AddThirdPartyCaveatWithRand adds a third-party caveat to the macaroon, using
// the given source of randomness for encrypting the caveat id.
var AddThirdPartyCaveatWithRand = (*Macaroon).addThirdPartyCaveatWithRand

// MaxPacketLen is the maximum allowed length of a packet in the macaroon
// serialization format.
var MaxPacketLen = maxPacketLen
