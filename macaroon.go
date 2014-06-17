package macaroon
import (
	"bytes"
	"encoding/json"
	"fmt"
	"crypto/sha256"
	"crypto/hmac"
)

// Macaroon holds a macaroon.
// See Fig. 7 of http://theory.stanford.edu/~ataly/Papers/macaroons.pdf
// for a description of the data contained within.
// Macaroons are mutable objects - use Clone as appropriate
// to avoid unwanted mutation.
type Macaroon struct {
	location string
	id       []byte
	caveats  []Caveat
	sig      []byte
}

// Caveat holds a first person or third party caveat.
type Caveat struct {
	location       string
	caveatId       []byte
	verificationId []byte
}

// IsThirdParty reports whether the caveat must be satisfied
// by some third party (if not, it's a first person caveat).
func (cav *Caveat) IsThirdParty() bool {
	return len(cav.verificationId) > 0
}

// New returns a new macaroon with the given root key,
// identifier and location.
func New(rootKey, id []byte, loc string) *Macaroon {
	m := &Macaroon{
		location: loc,
		id: []byte(id),
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
func (m *Macaroon) Id() []byte {
	return append([]byte(nil), m.id...)
}

// Signature returns the macaroon's signature.
func (m *Macaroon) Signature() []byte {
	return append([]byte(nil), m.sig...)
}

func (m *Macaroon) addCaveat(caveatId, verificationId []byte, loc string) {
	m.caveats = append(m.caveats, Caveat{
		location: loc,
		caveatId: caveatId,
		verificationId: verificationId,
	})
	sig := keyedHasher(m.sig)
	sig.Write(verificationId)
	sig.Write(caveatId)
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
func (m *Macaroon) AddFirstPartyCaveat(caveat string) {
	m.addCaveat([]byte(caveat), nil, "")
}

// ThirdPartyCaveatId holds the information encoded in
// a third-party caveat id.
type ThirdPartyCaveatId struct {
	RootKey []byte
	Caveat string
}

// DecryptThirdPartyCaveatId decrypts a third-party caveat
// id given the shared secret.
func DecryptThirdPartyCaveatId(secret, id []byte) (*ThirdPartyCaveatId, error) {
	plain, err := decrypt(secret, id)
	if err != nil {
		return nil, err
	}
	var c ThirdPartyCaveatId
	if err := json.Unmarshal(plain, &c); err != nil {
		return nil, fmt.Errorf("cannot unmarshal decrypted caveat id: %v", err)
	}
	return &c, nil
}

// AddThirdPartyCaveat adds a third-party caveat to the macaroon,
// using the given shared secret, caveat and location hint.
// It returns the caveat id of the third party macaroon.
func (m *Macaroon) AddThirdPartyCaveat(thirdPartySecret []byte, caveat string, loc string) (id []byte, err error) {
	nonce, err := newNonce()
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(ThirdPartyCaveatId{nonce[:], caveat})
	if err != nil {
		return nil, err
	}
	caveatId, err := encrypt(thirdPartySecret, data)
	if err != nil {
		return nil, err
	}
	verificationId, err := encrypt(m.sig, nonce[:])
	if err != nil {
		return nil, err
	}
	m.addCaveat(caveatId, verificationId, loc)
	return caveatId, nil
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
// first-party caveat - it may return an error the check
// cannot be made but the answer is not necessarily false.
// The discharge macaroons should be passed in discharges,
// keyed by macaroon id.
//
// Verify returns true if the verification succeeds; if returns
// (false, nil) if the verification fails, and (false, err) if
// the verification cannot be asserted (but may not be false).
func (m *Macaroon) Verify(rootKey []byte, check func(caveat string) (bool, error), discharges map[string]*Macaroon) (bool, error) {
	return m.verify(m.sig, rootKey, check, discharges)
}

func (m *Macaroon) verify(rootSig []byte, rootKey []byte, check func(caveat string) (bool, error), discharges map[string]*Macaroon) (bool, error) {
	if len(rootSig) == 0 {
		rootSig = m.sig
	}
	caveatSig := keyedHash(rootKey, m.id)
	for i, cav := range m.caveats {
		if cav.IsThirdParty() {
			cavKey, err := decrypt(caveatSig, cav.verificationId)
			if err != nil {
				return false, fmt.Errorf("failed to decrypt caveat %d signature: %v", i, err)
			}
			dm, ok := discharges[string(cav.caveatId)]
			if !ok {
				return false, fmt.Errorf("cannot find discharge macaroon for caveat %d", i)
			}
			ok, err = dm.verify(rootSig, cavKey, check, discharges)
			if !ok {
				return false, err
			}
		} else {
			ok, err := check(string(cav.caveatId))
			if !ok {
				return false, err
			}
		}
		sig := keyedHasher(caveatSig)
		sig.Write(cav.verificationId)
		sig.Write(cav.caveatId)
		caveatSig = sig.Sum(caveatSig[:0])
	}
	// TODO perhaps we should actually do this check before doing
	// all the potentially expensive caveat checks.
	boundSig := bindForRequest(rootSig, caveatSig)
	if !hmac.Equal(boundSig, m.sig) {
		return false, fmt.Errorf("signature mismatch after caveat verification")
	}
	return true, nil
}

type Verifier interface {
	Verify(m *Macaroon, rootKey []byte) (bool, error)
}
