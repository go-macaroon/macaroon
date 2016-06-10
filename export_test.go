package macaroon

var (
	AddThirdPartyCaveatWithRand = (*Macaroon).addThirdPartyCaveatWithRand
	MaxPacketV1Len              = maxPacketV1Len
)

// SetUnmarshaledAs sets the unmarshaledAs field of m to o;
// usually so that we can compare it for deep equality with
// another differently unmarshaled macaroon.
func (m *Macaroon) SetUnmarshaledAs(o MarshalOpts) {
	m.unmarshaledAs = o
}
