package httpbakery

import (
	"bytes"
	"code.google.com/p/go.crypto/nacl/box"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/rogpeppe/macaroon/bakery"
)

const keyLen = 32

// CaveatIdEncoder implements bakery.CaveatIdEncoder. It
// knows how to make caveat ids by communicating
// with the caveat id creation service served by DischargeHandler,
// and also how to create caveat ids using public key
// cryptography (also recognised by the DischargeHandler
// service).
type CaveatIdEncoder struct {
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

// NewCaveatIdEncoder returns a new CaveatIdEncoder key, which should
// have been created using the NACL box.GenerateKey function. The keys may be nil,
// in which case new keys will be generated automatically.
func NewCaveatIdEncoder(key *KeyPair) (*CaveatIdEncoder, error) {
	enc := &CaveatIdEncoder{}
	if key == nil {
		priv, pub, err := box.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("cannot generate key: %v", err)
		}
		enc.key.private, enc.key.public = *priv, *pub
	} else {
		enc.key = *key
	}
	return enc, nil
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
func (enc *CaveatIdEncoder) EncodeCaveatId(cav bakery.Caveat, rootKey []byte) (string, error) {
	if cav.Location == "" {
		return "", fmt.Errorf("cannot make caveat id for first party caveat")
	}
	var id *ThirdPartyCaveatId
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

func (enc *CaveatIdEncoder) newEncryptedCaveatId(cav bakery.Caveat, rootKey []byte, thirdPartyPub *[32]byte) (*ThirdPartyCaveatId, error) {
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
	return &ThirdPartyCaveatId{
		ThirdPartyPublicKey: thirdPartyPub[:],
		FirstPartyPublicKey: enc.key.public[:],
		Nonce:               nonce[:],
		Id:                  base64.StdEncoding.EncodeToString(sealed),
	}, nil
}

func (enc *CaveatIdEncoder) newStoredCaveatId(cav bakery.Caveat, rootKey []byte) (*ThirdPartyCaveatId, error) {
	// TODO(rog) fetch public key from service here, and use public
	// key encryption if available?

	// TODO(rog) check that the URL is https?
	// Is that really just smoke and mirrors though?
	// Are there advantages to having an unrestricted protocol?
	u := appendURLElem(cav.Location, "create")
	httpResp, err := http.PostForm(u, url.Values{
		"caveat": []string{cav.Condition},
	})
	if err != nil {
		return nil, fmt.Errorf("cannot create caveat id through %q: %v", u, err)
	}
	defer httpResp.Body.Close()
	data, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read caveat id from %q: %v", u, err)
	}
	var resp caveatIdResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("cannot unmarshal response from %q: %v", u, err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("remote error from %q: %v", u, resp.Error)
	}
	if resp.CaveatId == "" {
		return nil, fmt.Errorf("empty caveat id returned from %q", u)
	}
	return &ThirdPartyCaveatId{
		Id: resp.CaveatId,
	}, nil
}

func appendURLElem(u, elem string) string {
	if strings.HasSuffix(u, "/") {
		return u + elem
	}
	return u + "/" + elem
}

// ThirdPartyCaveatId defines the format
// of a third party caveat id. If ThirdPartyPublicKey
// is non-empty, then both FirstPartyPublicKey
// and Nonce must be set, and the id will have
// been encrypted with the third party public key
// and base64-encoded.
//
// If not, the Id holds an id that was created
// by the third party.
type ThirdPartyCaveatId struct {
	ThirdPartyPublicKey []byte `json:",omitempty"`
	FirstPartyPublicKey []byte `json:",omitempty"`
	Nonce               []byte `json:",omitempty"`
	Id                  string
}

// AddPublicKeyForLocation specifies that third party caveats
// for the given location will be encrypted with the given public
// key. If prefix is true, any locations with loc as a prefix will
// be also associated with the given key. The longest prefix
// match will be chosen.
// TODO(rog) perhaps string might be a better representation
// of public keys?
func (enc *CaveatIdEncoder) AddPublicKeyForLocation(loc string, prefix bool, key *[32]byte) {
	if len(key) != keyLen {
		panic("empty public key added")
	}
	enc.mu.Lock()
	defer enc.mu.Unlock()
	enc.publicKeys = append(enc.publicKeys, publicKeyRecord{
		location: loc,
		prefix:   prefix,
		key:      *key,
	})
}

func (enc *CaveatIdEncoder) publicKeyForLocation(loc string) *[32]byte {
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

func NewCaveatIdDecoder(store bakery.Storage, key *KeyPair) bakery.CaveatIdDecoder {
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
	var tpid ThirdPartyCaveatId
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

func (d *caveatIdDecoder) encryptedCaveatId(id ThirdPartyCaveatId) ([]byte, error) {
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
	str, err := d.store.Get("third-party-" + id)
	if err != nil {
		return nil, err
	}
	return []byte(str), nil
}
