package macaroon

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"unicode/utf8"
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
	if !utf8.Valid(m.id) {
		return nil, fmt.Errorf("macaroon id is not valid UTF-8")
	}
	mjson := macaroonJSON{
		Location:   m.location,
		Identifier: string(m.id),
		Signature:  hex.EncodeToString(m.sig[:]),
		Caveats:    make([]caveatJSON, len(m.caveats)),
	}
	for i, cav := range m.caveats {
		if !utf8.Valid(cav.Id) {
			return nil, fmt.Errorf("caveat id is not valid UTF-8")
		}
		mjson.Caveats[i] = caveatJSON{
			Location: cav.Location,
			CID:      string(cav.Id),
			VID:      base64.RawURLEncoding.EncodeToString(cav.VerificationId),
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
	if err := m.init([]byte(mjson.Identifier), mjson.Location); err != nil {
		return err
	}
	sig, err := hex.DecodeString(mjson.Signature)
	if err != nil {
		return fmt.Errorf("cannot decode macaroon signature %q: %v", m.sig, err)
	}
	if len(sig) != hashLen {
		return fmt.Errorf("signature has unexpected length %d", len(sig))
	}
	copy(m.sig[:], sig)
	m.caveats = m.caveats[:0]
	for _, cav := range mjson.Caveats {
		vid, err := base64Decode(cav.VID)
		if err != nil {
			return fmt.Errorf("cannot decode verification id %q: %v", cav.VID, err)
		}
		if err := m.appendCaveat([]byte(cav.CID), vid, cav.Location); err != nil {
			return err
		}
	}
	return nil
}

// MarshalBinary implements encoding.BinaryMarshaler.
func (m *Macaroon) MarshalBinary() ([]byte, error) {
	return m.appendBinary(nil)
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

// unmarshalBinaryNoCopy is the internal implementation of
// UnmarshalBinary. It differs in that it does not copy the
// data. It returns the data after the end of the macaroon.
func (m *Macaroon) unmarshalBinaryNoCopy(data []byte) ([]byte, error) {
	var err error

	loc, err := expectPacketV1(data, fieldNameLocation)
	if err != nil {
		return nil, err
	}
	data = data[loc.totalLen:]
	m.location = string(loc.data)
	id, err := expectPacketV1(data, fieldNameIdentifier)
	if err != nil {
		return nil, err
	}
	data = data[id.totalLen:]
	m.id = id.data
	var cav Caveat
	for {
		p, err := parsePacketV1(data)
		if err != nil {
			return nil, err
		}
		data = data[p.totalLen:]
		switch field := string(p.fieldName); field {
		case fieldNameSignature:
			// At the end of the caveats we find the signature.
			if cav.Id != nil {
				m.caveats = append(m.caveats, cav)
			}
			if len(p.data) != hashLen {
				return nil, fmt.Errorf("signature has unexpected length %d", len(p.data))
			}
			copy(m.sig[:], p.data)
			return data, nil
		case fieldNameCaveatId:
			if cav.Id != nil {
				m.caveats = append(m.caveats, cav)
				cav = Caveat{}
			}
			cav.Id = p.data
		case fieldNameVerificationId:
			if cav.VerificationId != nil {
				return nil, fmt.Errorf("repeated field %q in caveat", fieldNameVerificationId)
			}
			cav.VerificationId = p.data
		case fieldNameCaveatLocation:
			if cav.Location != "" {
				return nil, fmt.Errorf("repeated field %q in caveat", fieldNameLocation)
			}
			cav.Location = string(p.data)
		default:
			return nil, fmt.Errorf("unexpected field %q", field)
		}
	}
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (m *Macaroon) UnmarshalBinary(data []byte) error {
	data = append([]byte(nil), data...)
	_, err := m.unmarshalBinaryNoCopy(data)
	return err
}

func expectPacketV1(data []byte, kind string) (packetV1, error) {
	p, err := parsePacketV1(data)
	if err != nil {
		return packetV1{}, err
	}
	if field := string(p.fieldName); field != kind {
		return packetV1{}, fmt.Errorf("unexpected field %q; expected %s", field, kind)
	}
	return p, nil
}

// appendBinary appends the binary encoding of m to data.
func (m *Macaroon) appendBinary(data []byte) ([]byte, error) {
	var ok bool
	data, ok = appendPacketV1(data, fieldNameLocation, []byte(m.location))
	if !ok {
		return nil, fmt.Errorf("failed to append location to macaroon, packet is too long")
	}
	data, ok = appendPacketV1(data, fieldNameIdentifier, m.id)
	if !ok {
		return nil, fmt.Errorf("failed to append identifier to macaroon, packet is too long")
	}
	for _, cav := range m.caveats {
		data, ok = appendPacketV1(data, fieldNameCaveatId, cav.Id)
		if !ok {
			return nil, fmt.Errorf("failed to append caveat id to macaroon, packet is too long")
		}
		if cav.VerificationId == nil {
			continue
		}
		data, ok = appendPacketV1(data, fieldNameVerificationId, cav.VerificationId)
		if !ok {
			return nil, fmt.Errorf("failed to append verification id to macaroon, packet is too long")
		}
		data, ok = appendPacketV1(data, fieldNameCaveatLocation, []byte(cav.Location))
		if !ok {
			return nil, fmt.Errorf("failed to append verification id to macaroon, packet is too long")
		}
	}
	data, ok = appendPacketV1(data, fieldNameSignature, m.sig[:])
	if !ok {
		return nil, fmt.Errorf("failed to append signature to macaroon, packet is too long")
	}
	return data, nil
}

// Slice defines a collection of macaroons. By convention, the
// first macaroon in the slice is a primary macaroon and the rest
// are discharges for its third party caveats.
type Slice []*Macaroon

// MarshalBinary implements encoding.BinaryMarshaler.
func (s Slice) MarshalBinary() ([]byte, error) {
	var data []byte
	var err error
	for _, m := range s {
		data, err = m.appendBinary(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal macaroon %q: %v", m.Id(), err)
		}
	}
	return data, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (s *Slice) UnmarshalBinary(data []byte) error {
	// Prevent the internal data structures from holding onto the
	// slice by copying it first.
	data = append([]byte(nil), data...)
	*s = (*s)[:0]
	for len(data) > 0 {
		var m Macaroon
		rest, err := m.unmarshalBinaryNoCopy(data)
		if err != nil {
			return fmt.Errorf("cannot unmarshal macaroon: %v", err)
		}
		*s = append(*s, &m)
		data = rest
	}
	return nil
}

// base64Decode decodes base64 data that might be missing trailing
// pad characters.
func base64Decode(b64String string) ([]byte, error) {
	if data, err := base64.StdEncoding.DecodeString(b64String); err == nil {
		return data, nil
	}
	return base64.RawURLEncoding.DecodeString(b64String)
}
