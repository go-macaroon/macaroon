package macaroon

import (
	"encoding/base64"
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
	fieldCaveatLocation = "location"
)

var (
	fieldLocationBytes       = []byte("location")
	fieldIdentifierBytes     = []byte("identifier")
	fieldSignatureBytes      = []byte("signature")
	fieldCaveatIdBytes       = []byte("cid")
	fieldVerificationIdBytes = []byte("vid")
	fieldCaveatLocationBytes = []byte("cl")
)

// macaroonJSON defines the JSON format for macaroons.
type macaroonJSON struct {
	Caveats    []caveatJSON `json:"caveats"`
	Location   string       `json:"location"`
	Identifier string       `json:"identifier"`
	Signature  string       `json:"signature"` // hex-encoded
}

// caveatJSON defines the JSON format for caveats within a macaroon.
type caveatJSON struct {
	CID      string `json:"cid"`
	VID      string `json:"vid,omitempty"`
	Location string `json:"cl,omitempty"`
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
			VID:      base64.StdEncoding.EncodeToString(m.dataBytes(cav.verificationId)),
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
		vid, err := base64.StdEncoding.DecodeString(cav.VID)
		if err != nil {
			return fmt.Errorf("cannot decode verification id %q: %v", cav.VID, err)
		}
		if _, err := m.appendCaveat(cav.CID, vid, cav.Location); err != nil {
			return err
		}
	}
	return nil
}

// MarshalBinary implements encoding.BinaryMarshaler.
func (m *Macaroon) MarshalBinary() ([]byte, error) {
	data := make([]byte, len(m.data), len(m.data)+len(m.sig))
	copy(data, m.data)
	data, _, ok := rawAppendPacket(data, fieldSignature, m.sig)
	if !ok {
		panic("cannot append signature")
	}
	return data, nil
}

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

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (m *Macaroon) UnmarshalBinary(data []byte) error {
	m.data = append([]byte(nil), data...)

	var err error
	var start int

	start, m.location, err = m.expectPacket(0, fieldLocation)
	if err != nil {
		return err
	}
	start, m.id, err = m.expectPacket(start, fieldIdentifier)
	if err != nil {
		return err
	}
	var cav caveat
	for {
		p, err := m.parsePacket(start)
		if err != nil {
			return err
		}
		start += p.len()
		switch field := string(m.fieldName(p)); field {
		case fieldSignature:
			// At the end of the caveats we find the signature.
			if cav.caveatId.len() != 0 {
				m.caveats = append(m.caveats, cav)
			}
			// Remove the signature from data.
			m.data = m.data[0:p.start]
			m.sig = append([]byte(nil), m.dataBytes(p)...)
			return nil
		case fieldCaveatId:
			if cav.caveatId.len() != 0 {
				m.caveats = append(m.caveats, cav)
			}
			cav.caveatId = p
		case fieldVerificationId:
			if cav.verificationId.len() != 0 {
				return fmt.Errorf("repeated field %q in caveat", fieldVerificationId)
			}
			cav.verificationId = p
		case fieldCaveatLocation:
			if cav.location.len() != 0 {
				return fmt.Errorf("repeated field %q in caveat", fieldLocation)
			}
			cav.location = p
		default:
			return fmt.Errorf("unexpected field %q", field)
		}
	}
	return nil
}

func (m *Macaroon) expectPacket(start int, kind string) (int, packet, error) {
	p, err := m.parsePacket(start)
	if err != nil {
		return 0, packet{}, err
	}
	if field := string(m.fieldName(p)); field != kind {
		return 0, packet{}, fmt.Errorf("unexpected field %q; expected %s", field, kind)
	}
	return start + p.len(), p, nil
}
