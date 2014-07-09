package macaroon

// Signature returns the macaroon's signature.
func (m *Macaroon) Data() []byte {
	return append([]byte(nil), m.data...)
}
