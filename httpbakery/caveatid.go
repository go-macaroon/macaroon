package httpbakery

import (
	"bytes"
	"code.google.com/p/go.crypto/nacl/box"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/rogpeppe/macaroon/bakery"
)

const keyLen = 32

// caveatIdEncoder implements bakery.CaveatIdEncoder. It
// knows how to make caveat ids by communicating
// with the caveat id creation service served by DischargeHandler,
// and also how to create caveat ids using public key
// cryptography (also recognised by the DischargeHandler
// service).
type caveatIdEncoder struct {
	key KeyPair

	// mu guards the fields following it.
	mu sync.Mutex

	// TODO(rog) use a more efficient data structure
	publicKeys []publicKeyRecord
}

type publicKeyRecord struct {
	location string
	prefix   bool
	key      [32]byte
}

type KeyPair struct {
	public  [32]byte
	private [32]byte
}

// TODO(rog) marshal/unmarshal functions for KeyPair

func GenerateKey() (*KeyPair, error) {
	var key KeyPair
	priv, pub, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	key.public = *pub
	key.private = *priv
	return &key, nil
}

// newCaveatIdEncoder returns a new caveatIdEncoder using key, which should
// have been created using GenerateKey.
func newCaveatIdEncoder(key *KeyPair) *caveatIdEncoder {
	return &caveatIdEncoder{
		key: *key,
	}
}

type caveatIdResponse struct {
	CaveatId string
	Error    string
}

type caveatIdSealed struct {
	Condition string
	Secret    []byte
}

// EncodeCaveatId implements bakery.CaveatIdEncoder.EncodeCaveatId.
// This is the client side of DischargeHandler's /create endpoint.
func (enc *caveatIdEncoder) EncodeCaveatId(cav bakery.Caveat, rootKey []byte) (string, error) {
	if cav.Location == "" {
		return "", fmt.Errorf("cannot make caveat id for first party caveat")
	}
	var id *thirdPartyCaveatId
	var err error
	thirdPartyPub := enc.publicKeyForLocation(cav.Location)
	if thirdPartyPub != nil {
		id, err = enc.newEncryptedCaveatId(cav, rootKey, thirdPartyPub)
	} else {
		id, err = enc.newStoredCaveatId(cav, rootKey)
	}
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(id)
	if err != nil {
		return "", fmt.Errorf("cannot marshal %#v: %v", id, err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func (enc *caveatIdEncoder) newEncryptedCaveatId(cav bakery.Caveat, rootKey []byte, thirdPartyPub *[32]byte) (*thirdPartyCaveatId, error) {
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("cannot generate random number for nonce: %v", err)
	}
	plain := thirdPartyCaveatIdRecord{
		RootKey:   rootKey,
		Condition: cav.Condition,
	}
	plainData, err := json.Marshal(&plain)
	if err != nil {
		return nil, fmt.Errorf("cannot marshal %#v: %v", &plain, err)
	}
	sealed := box.Seal(nil, plainData, &nonce, thirdPartyPub, &enc.key.private)
	return &thirdPartyCaveatId{
		ThirdPartyPublicKey: thirdPartyPub[:],
		FirstPartyPublicKey: enc.key.public[:],
		Nonce:               nonce[:],
		Id:                  base64.StdEncoding.EncodeToString(sealed),
	}, nil
}

func (enc *caveatIdEncoder) newStoredCaveatId(cav bakery.Caveat, rootKey []byte) (*thirdPartyCaveatId, error) {
	// TODO(rog) fetch public key from service here, and use public
	// key encryption if available?

	// TODO(rog) check that the URL is https?
	// Is that really just smoke and mirrors though?
	// Are there advantages to having an unrestricted protocol?
	u := appendURLElem(cav.Location, "create")

	var resp caveatIdResponse
	if err := postFormJSON(u, url.Values{
		"condition": {cav.Condition},
		"root-key":  {base64.StdEncoding.EncodeToString(rootKey)},
	}, &resp); err != nil {
		return nil, fmt.Errorf("cannot create caveat id through %q: %v", u, err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("remote error from %q: %v", u, resp.Error)
	}
	if resp.CaveatId == "" {
		return nil, fmt.Errorf("empty caveat id returned from %q", u)
	}
	return &thirdPartyCaveatId{
		Id: resp.CaveatId,
	}, nil
}

func appendURLElem(u, elem string) string {
	if strings.HasSuffix(u, "/") {
		return u + elem
	}
	return u + "/" + elem
}

// thirdPartyCaveatId defines the format
// of a third party caveat id. If ThirdPartyPublicKey
// is non-empty, then both FirstPartyPublicKey
// and Nonce must be set, and the id will have
// been encrypted with the third party public key
// and base64-encoded.
//
// If not, the Id holds an id that was created
// by the third party.
type thirdPartyCaveatId struct {
	ThirdPartyPublicKey []byte `json:",omitempty"`
	FirstPartyPublicKey []byte `json:",omitempty"`
	Nonce               []byte `json:",omitempty"`
	Id                  string
}

func (enc *caveatIdEncoder) addPublicKeyForLocation(loc string, prefix bool, key *[32]byte) {
	enc.mu.Lock()
	defer enc.mu.Unlock()
	enc.publicKeys = append(enc.publicKeys, publicKeyRecord{
		location: loc,
		prefix:   prefix,
		key:      *key,
	})
}

func (enc *caveatIdEncoder) publicKeyForLocation(loc string) *[32]byte {
	enc.mu.Lock()
	defer enc.mu.Unlock()
	var (
		longestPrefix    string
		longestPrefixKey *[32]byte // public key associated with longest prefix
	)
	for i := len(enc.publicKeys) - 1; i >= 0; i-- {
		k := enc.publicKeys[i]
		if k.location == loc && !k.prefix {
			return &k.key
		}
		if !k.prefix {
			continue
		}
		if strings.HasPrefix(loc, k.location) && len(k.location) > len(longestPrefix) {
			longestPrefix = k.location
			longestPrefixKey = &k.key
		}
	}
	if len(longestPrefix) == 0 {
		return nil
	}
	return longestPrefixKey
}

type caveatIdDecoder struct {
	store bakery.Storage
	key   *KeyPair
}

func newCaveatIdDecoder(store bakery.Storage, key *KeyPair) bakery.CaveatIdDecoder {
	return &caveatIdDecoder{
		store: store,
		key:   key,
	}
}

func (d *caveatIdDecoder) DecodeCaveatId(id string) (rootKey []byte, condition string, err error) {
	data, err := base64.StdEncoding.DecodeString(id)
	if err != nil {
		return nil, "", fmt.Errorf("cannot base64-decode caveat id: %v", err)
	}
	var tpid thirdPartyCaveatId
	if err := json.Unmarshal(data, &tpid); err != nil {
		return nil, "", fmt.Errorf("cannot unmarshal caveat id: %v", err)
	}
	var recordData []byte

	if tpid.ThirdPartyPublicKey != nil {
		recordData, err = d.encryptedCaveatId(tpid)
	} else {
		recordData, err = d.storedCaveatId(tpid.Id)
	}
	if err != nil {
		return nil, "", err
	}
	var record thirdPartyCaveatIdRecord
	if err := json.Unmarshal(recordData, &record); err != nil {
		return nil, "", fmt.Errorf("cannot decode third party caveat record: %v", err)
	}
	return record.RootKey, record.Condition, nil
}

func (d *caveatIdDecoder) encryptedCaveatId(id thirdPartyCaveatId) ([]byte, error) {
	if d.key == nil {
		return nil, fmt.Errorf("no public key for caveat id decryption")
	}
	if !bytes.Equal(d.key.public[:], id.ThirdPartyPublicKey) {
		return nil, fmt.Errorf("public key mismatch")
	}
	var nonce [24]byte
	if len(id.Nonce) != len(nonce) {
		return nil, fmt.Errorf("bad nonce length")
	}
	copy(nonce[:], id.Nonce)

	var firstPartyPublicKey [32]byte
	if len(id.FirstPartyPublicKey) != len(firstPartyPublicKey) {
		return nil, fmt.Errorf("bad public key length")
	}
	copy(firstPartyPublicKey[:], id.FirstPartyPublicKey)

	sealed, err := base64.StdEncoding.DecodeString(id.Id)
	if err != nil {
		return nil, fmt.Errorf("cannot base64-decode encrypted caveat id", err)
	}
	out, ok := box.Open(nil, sealed, &nonce, &firstPartyPublicKey, &d.key.private)
	if !ok {
		return nil, fmt.Errorf("decryption of public-key encrypted caveat id failed")
	}
	return out, nil
}

func (d *caveatIdDecoder) storedCaveatId(id string) ([]byte, error) {
	str, err := d.store.Get(id)
	if err != nil {
		return nil, err
	}
	return []byte(str), nil
}
