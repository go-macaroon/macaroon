// The macaroon package implements macaroons as described in
// the paper "Macaroons: Cookies with Contextual Caveats for
// Decentralized Authorization in the Cloud"
// (http://theory.stanford.edu/~ataly/Papers/macaroons.pdf)
package macaroon

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Macaroon holds a macaroon.
// See Fig. 7 of http://theory.stanford.edu/~ataly/Papers/macaroons.pdf
// for a description of the data contained within.
// Macaroons are mutable objects - use Clone as appropriate
// to avoid unwanted mutation.
type Macaroon struct {
	location string
	id       string
	caveats  []Caveat
	sig      []byte
}

// Caveat holds a first person or third party caveat.
type Caveat struct {
	location       string
	caveatId       string
	verificationId []byte
}

// macaroonJSON defines the JSON format for macaroons.
type macaroonJSON struct {
	Caveats    []Caveat `json:"caveats"`
	Location   string   `json:"location"`
	Identifier string   `json:"identifier"`
	Signature  string   `json:"signature"` // hex-encoded
}

// caveatJSON defines the JSON format for caveats within a macaroon.
type caveatJSON struct {
	Location string `json:"location"`
	CID      string `json:"cid"`
	VID      string `json:"vid"`
}

// MarshalJSON implements json.Marshaler.
func (cav *Caveat) MarshalJSON() ([]byte, error) {
	cavJSON := caveatJSON{
		Location: cav.location,
		CID:      cav.caveatId,
		VID:      hex.EncodeToString(cav.verificationId),
	}
	data, err := json.Marshal(cavJSON)
	if err != nil {
		return nil, fmt.Errorf("cannot marshal json data: %v", err)
	}
	return data, nil
}

// unmarshalJSON implements json.Unmarshaler.
func (cav *Caveat) UnmarshalJSON(jsonData []byte) error {
	var cavJSON caveatJSON
	err := json.Unmarshal(jsonData, &cavJSON)
	if err != nil {
		return fmt.Errorf("cannot decode caveat id %q: %v", cavJSON.CID, err)
	}
	cav.location = cavJSON.Location
	cav.caveatId = cavJSON.CID
	cav.verificationId, err = hex.DecodeString(cavJSON.VID)
	if err != nil {
		return fmt.Errorf("cannot decode verfification id %q: %v", cavJSON.VID, err)
	}
	return nil
}

// IsThirdParty reports whether the caveat must be satisfied
// by some third party (if not, it's a first person caveat).
func (cav *Caveat) IsThirdParty() bool {
	return len(cav.verificationId) > 0
}

// New returns a new macaroon with the given root key,
// identifier and location.
func New(rootKey []byte, id, loc string) *Macaroon {
	m := &Macaroon{
		location: loc,
		id:       id,
	}
	m.sig = keyedHash(rootKey, m.id)
	return m
}

// Clone returns a copy of the receiving macaroon.
func (m *Macaroon) Clone() *Macaroon {
	m1 := *m
	m1.caveats = make([]Caveat, len(m.caveats))
	copy(m1.caveats, m.caveats)
	return &m1
}

// Location returns the macaroon's location hint. This is
// not verified as part of the macaroon.
func (m *Macaroon) Location() string {
	return m.location
}

// Id returns the id of the macaroon. This can hold
// arbitrary information.
func (m *Macaroon) Id() string {
	return m.id
}

// Signature returns the macaroon's signature.
func (m *Macaroon) Signature() []byte {
	return append([]byte(nil), m.sig...)
}

// Caveats returns the macaroon's caveats.
// This method will probably change, and it's important not to change the returned caveat.
func (m *Macaroon) Caveats() []Caveat {
	return m.caveats
}

func (m *Macaroon) addCaveat(caveatId string, verificationId []byte, loc string) {
	m.caveats = append(m.caveats, Caveat{
		location:       loc,
		caveatId:       caveatId,
		verificationId: verificationId,
	})
	sig := keyedHasher(m.sig)
	sig.Write(verificationId)
	sig.Write([]byte(caveatId))
	m.sig = sig.Sum(nil)
}

// Bind prepares the macaroon for being used to discharge the
// macaroon with the given rootSig. This must be
// used before it is used in the discharges argument to Verify.
func (m *Macaroon) Bind(rootSig []byte) {
	m.sig = bindForRequest(rootSig, m.sig)
}

// AddFirstPartyCaveat adds a caveat that will be verified
// by the target service.
func (m *Macaroon) AddFirstPartyCaveat(caveatId string) {
	m.addCaveat(caveatId, nil, "")
}

// AddThirdPartyCaveat adds a third-party caveat to the macaroon,
// using the given shared root key, caveat id and location hint.
// The caveat id should encode the root key in some
// way, either by encrypting it with a key known to the third party
// or by holding a reference to it stored in the third party's
// storage.
func (m *Macaroon) AddThirdPartyCaveat(rootKey []byte, caveatId string, loc string) error {
	verificationId, err := encrypt(m.sig, rootKey)
	if err != nil {
		return err
	}
	m.addCaveat(caveatId, verificationId, loc)
	return nil
}

// bndForRequest binds the given macaroon
// to the given signature of its parent macaroon.
func bindForRequest(rootSig, dischargeSig []byte) []byte {
	if bytes.Equal(rootSig, dischargeSig) {
		return rootSig
	}
	sig := sha256.New()
	sig.Write(rootSig)
	sig.Write(dischargeSig)
	return sig.Sum(nil)
}

// Verify verifies that the receiving macaroon is valid.
// The root key must be the same that the macaroon was originally
// minted with. The check function is called to verify each
// first-party caveat - it should return an error if the
// condition is not met.
//
// The discharge macaroons should be provided in discharges.
//
// Verify returns true if the verification succeeds; if returns
// (false, nil) if the verification fails, and (false, err) if
// the verification cannot be asserted (but may not be false).
//
// TODO(rog) is there a possible DOS attack that can cause this
// function to infinitely recurse?
func (m *Macaroon) Verify(rootKey []byte, check func(caveat string) error, discharges []*Macaroon) error {
	// TODO(rog) consider distinguishing between classes of
	// check error - some errors may be resolved by minting
	// a new macaroon; others may not.
	return m.verify(m.sig, rootKey, check, discharges)
}

func (m *Macaroon) verify(rootSig []byte, rootKey []byte, check func(caveat string) error, discharges []*Macaroon) error {
	if len(rootSig) == 0 {
		rootSig = m.sig
	}
	caveatSig := keyedHash(rootKey, m.id)
	for i, cav := range m.caveats {
		if cav.IsThirdParty() {
			cavKey, err := decrypt(caveatSig, cav.verificationId)
			if err != nil {
				return fmt.Errorf("failed to decrypt caveat %d signature: %v", i, err)
			}
			// We choose an arbitrary error from one of the
			// possible discharge macaroon verifications
			// if there's more than one discharge macaroon
			// with the required id.
			var verifyErr error
			found := false
			for _, dm := range discharges {
				if dm.Id() != cav.caveatId {
					continue
				}
				found = true
				verifyErr = dm.verify(rootSig, cavKey, check, discharges)
				if verifyErr == nil {
					break
				}
			}
			if !found {
				return fmt.Errorf("cannot find discharge macaroon for caveat %q", cav.caveatId)
			}
			if verifyErr != nil {
				return verifyErr
			}
		} else {
			if err := check(cav.caveatId); err != nil {
				return err
			}
		}
		sig := keyedHasher(caveatSig)
		sig.Write(cav.verificationId)
		sig.Write([]byte(cav.caveatId))
		caveatSig = sig.Sum(caveatSig[:0])
	}
	// TODO perhaps we should actually do this check before doing
	// all the potentially expensive caveat checks.
	boundSig := bindForRequest(rootSig, caveatSig)
	if !hmac.Equal(boundSig, m.sig) {
		return fmt.Errorf("signature mismatch after caveat verification")
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (m *Macaroon) MarshalJSON() ([]byte, error) {
	mjson := macaroonJSON{
		Location:   m.Location(),
		Identifier: m.id,
		Signature:  hex.EncodeToString(m.sig),
		Caveats:    m.caveats,
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
	m.location = mjson.Location
	m.id = mjson.Identifier
	m.sig, err = hex.DecodeString(mjson.Signature)
	if err != nil {
		return fmt.Errorf("cannot decode macaroon signature %q: %v", m.sig, err)
	}
	m.caveats = mjson.Caveats
	return nil
}

type Verifier interface {
	Verify(m *Macaroon, rootKey []byte) (bool, error)
}
