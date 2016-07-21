package macaroon

var (
	AddThirdPartyCaveatWithRand = (*Macaroon).addThirdPartyCaveatWithRand
	MaxPacketV1Len              = maxPacketV1Len
)

// SetVersion sets the version field of m to v;
// usually so that we can compare it for deep equality with
// another differently unmarshaled macaroon.
func (m *Macaroon) SetVersion(v Version) {
	m.version = v
}
