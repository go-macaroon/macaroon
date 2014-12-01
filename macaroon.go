// The macaroon package implements macaroons as described in
// the paper "Macaroons: Cookies with Contextual Caveats for
// Decentralized Authorization in the Cloud"
// (http://theory.stanford.edu/~ataly/Papers/macaroons.pdf)
package macaroon

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
)

// Macaroon holds a macaroon.
// See Fig. 7 of http://theory.stanford.edu/~ataly/Papers/macaroons.pdf
// for a description of the data contained within.
// Macaroons are mutable objects - use Clone as appropriate
// to avoid unwanted mutation.
type Macaroon struct {
	// data holds the binary-marshalled form
	// of the macaroon data.
	data []byte

	location packet
	id       packet
	caveats  []caveat
	sig      []byte
}

// caveat holds a first person or third party caveat.
type caveat struct {
	location       packet
	caveatId       packet
	verificationId packet
}

type Caveat struct {
	Id       string
	Location string
}

// isThirdParty reports whether the caveat must be satisfied
// by some third party (if not, it's a first person caveat).
func (cav *caveat) isThirdParty() bool {
	return cav.verificationId.len() > 0
}

// New returns a new macaroon with the given root key,
// identifier and location.
func New(rootKey []byte, id, loc string) (*Macaroon, error) {
	var m Macaroon
	if err := m.init(id, loc); err != nil {
		return nil, err
	}
	m.sig = keyedHash(rootKey, m.dataBytes(m.id))
	return &m, nil
}

func (m *Macaroon) init(id, loc string) error {
	var ok bool
	m.location, ok = m.appendPacket(fieldLocation, []byte(loc))
	if !ok {
		return fmt.Errorf("macaroon location too big")
	}
	m.id, ok = m.appendPacket(fieldIdentifier, []byte(id))
	if !ok {
		return fmt.Errorf("macaroon identifier too big")
	}
	return nil
}

// Clone returns a copy of the receiving macaroon.
func (m *Macaroon) Clone() *Macaroon {
	m1 := *m
	// Ensure that if any data is appended to the new
	// macaroon, it will copy data and caveats.
	m1.data = m1.data[0:len(m1.data):len(m1.data)]
	m1.caveats = m1.caveats[0:len(m1.caveats):len(m1.caveats)]
	m1.sig = append([]byte(nil), m.sig...)
	return &m1
}

// Location returns the macaroon's location hint. This is
// not verified as part of the macaroon.
func (m *Macaroon) Location() string {
	return m.dataStr(m.location)
}

// Id returns the id of the macaroon. This can hold
// arbitrary information.
func (m *Macaroon) Id() string {
	return m.dataStr(m.id)
}

// Signature returns the macaroon's signature.
func (m *Macaroon) Signature() []byte {
	return append([]byte(nil), m.sig...)
}

// Caveats returns the macaroon's caveats.
// This method will probably change, and it's important not to change the returned caveat.
func (m *Macaroon) Caveats() []Caveat {
	caveats := make([]Caveat, len(m.caveats))
	for i, cav := range m.caveats {
		caveats[i] = Caveat{
			Id:       m.dataStr(cav.caveatId),
			Location: m.dataStr(cav.location),
		}
	}
	return caveats
}

// appendCaveat appends a caveat without modifying the macaroon's signature.
func (m *Macaroon) appendCaveat(caveatId string, verificationId []byte, loc string) (*caveat, error) {
	var cav caveat
	var ok bool
	if caveatId != "" {
		cav.caveatId, ok = m.appendPacket(fieldCaveatId, []byte(caveatId))
		if !ok {
			return nil, fmt.Errorf("caveat identifier too big")
		}
	}
	if len(verificationId) > 0 {
		cav.verificationId, ok = m.appendPacket(fieldVerificationId, verificationId)
		if !ok {
			return nil, fmt.Errorf("caveat verification id too big")
		}
	}
	if loc != "" {
		cav.location, ok = m.appendPacket(fieldCaveatLocation, []byte(loc))
		if !ok {
			return nil, fmt.Errorf("caveat location too big")
		}
	}
	m.caveats = append(m.caveats, cav)
	return &m.caveats[len(m.caveats)-1], nil
}

func (m *Macaroon) addCaveat(caveatId string, verificationId []byte, loc string) error {
	cav, err := m.appendCaveat(caveatId, verificationId, loc)
	if err != nil {
		return err
	}
	sig := keyedHasher(m.sig)
	sig.Write(m.dataBytes(cav.verificationId))
	sig.Write(m.dataBytes(cav.caveatId))
	m.sig = sig.Sum(m.sig[:0])
	return nil
}

// Bind prepares the macaroon for being used to discharge the
// macaroon with the given signature sig. This must be
// used before it is used in the discharges argument to Verify.
func (m *Macaroon) Bind(sig []byte) {
	m.sig = bindForRequest(sig, m.sig)
}

// AddFirstPartyCaveat adds a caveat that will be verified
// by the target service.
func (m *Macaroon) AddFirstPartyCaveat(caveatId string) error {
	return m.addCaveat(caveatId, nil, "")
}

// AddThirdPartyCaveat adds a third-party caveat to the macaroon,
// using the given shared root key, caveat id and location hint.
// The caveat id should encode the root key in some
// way, either by encrypting it with a key known to the third party
// or by holding a reference to it stored in the third party's
// storage.
func (m *Macaroon) AddThirdPartyCaveat(rootKey []byte, caveatId string, loc string) error {
	return m.addThirdPartyCaveatWithRand(rootKey, caveatId, loc, rand.Reader)
}

func (m *Macaroon) addThirdPartyCaveatWithRand(rootKey []byte, caveatId string, loc string, r io.Reader) error {
	verificationId, err := encrypt(m.sig, rootKey, r)
	if err != nil {
		return err
	}
	return m.addCaveat(caveatId, verificationId, loc)
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
	caveatSig := keyedHash(rootKey, m.dataBytes(m.id))
	for i, cav := range m.caveats {
		if cav.isThirdParty() {
			cavKey, err := decrypt(caveatSig, m.dataBytes(cav.verificationId))
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
				if !bytes.Equal(dm.dataBytes(dm.id), m.dataBytes(cav.caveatId)) {
					continue
				}
				found = true
				verifyErr = dm.verify(rootSig, cavKey, check, discharges)
				if verifyErr == nil {
					break
				}
			}
			if !found {
				return fmt.Errorf("cannot find discharge macaroon for caveat %q", m.dataBytes(cav.caveatId))
			}
			if verifyErr != nil {
				return verifyErr
			}
		} else {
			if err := check(string(m.dataBytes(cav.caveatId))); err != nil {
				return err
			}
		}
		sig := keyedHasher(caveatSig)
		sig.Write(m.dataBytes(cav.verificationId))
		sig.Write(m.dataBytes(cav.caveatId))
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

type Verifier interface {
	Verify(m *Macaroon, rootKey []byte) (bool, error)
}
