package macaroon

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// field names, as defined in libmacaroons
const (
	fieldLocation       = "location"
	fieldIdentifier     = "identifier"
	fieldSignature      = "signature"
	fieldCaveatId       = "cid"
	fieldVerificationId = "vid"
	fieldCaveatLocation = "cl"
)

// The binary format of a macaroon is as follows.
// Each identifier repesents a packet.
//
// location
// identifier
// (
//	caveatId?
//	verificationId?
//	caveatLocation?
// )*
// signature

// macaroonJSON defines the JSON format for macaroons.
type macaroonJSON struct {
	Caveats    []caveatJSON `json:"caveats"`
	Location   string       `json:"location"`
	Identifier string       `json:"identifier"`
	Signature  string       `json:"signature"` // hex-encoded
}

// caveatJSON defines the JSON format for caveats within a macaroon.
type caveatJSON struct {
	Location string `json:"location"`
	CID      string `json:"cid"`
	VID      string `json:"vid"`
}

// MarshalJSON implements json.Marshaler.
func (m *Macaroon) MarshalJSON() ([]byte, error) {
	mjson := macaroonJSON{
		Location:   m.Location(),
		Identifier: m.dataStr(m.id),
		Signature:  hex.EncodeToString(m.sig),
		Caveats:    make([]caveatJSON, len(m.caveats)),
	}
	for i, cav := range m.caveats {
		mjson.Caveats[i] = caveatJSON{
			Location: m.dataStr(cav.location),
			CID:      m.dataStr(cav.caveatId),
			VID:      m.dataStr(cav.verificationId),
		}
	}
	data, err := json.Marshal(mjson)
	if err != nil {
		return nil, fmt.Errorf("cannot marshal json data: %v", err)
	}
	return data, nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (m *Macaroon) UnmarshalJSON(jsonData []byte) error {
	var mjson macaroonJSON
	err := json.Unmarshal(jsonData, &mjson)
	if err != nil {
		return fmt.Errorf("cannot unmarshal json data: %v", err)
	}
	if err := m.init(mjson.Identifier, mjson.Location); err != nil {
		return err
	}
	m.sig, err = hex.DecodeString(mjson.Signature)
	if err != nil {
		return fmt.Errorf("cannot decode macaroon signature %q: %v", m.sig, err)
	}
	m.caveats = m.caveats[:0]
	for _, cav := range mjson.Caveats {
		vid, err := hex.DecodeString(cav.VID)
		if err != nil {
			return fmt.Errorf("cannot decode verification id %q: %v", cav.VID, err)
		}
		if _, err := m.appendCaveat(cav.CID, vid, cav.Location); err != nil {
			return err
		}
	}
	return nil
}
