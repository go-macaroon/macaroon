package macaroon

// Data returns the macaroon's data.
func (m *Macaroon) Data() []byte {
	return m.data
}
