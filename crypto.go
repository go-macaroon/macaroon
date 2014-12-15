package macaroon

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"hash"
	"io"

	"golang.org/x/crypto/nacl/secretbox"
)

func keyedHash(key, text []byte) []byte {
	h := keyedHasher(key)
	h.Write([]byte(text))
	return h.Sum(nil)
}

func keyedHasher(key []byte) hash.Hash {
	return hmac.New(sha256.New, key)
}

func makeKey(key []byte) *[keyLen]byte {
	if len(key) < keyLen {
		var h [keyLen]byte
		copy(h[:], key)
		return &h
	}
	h := sha256.Sum256(key)
	return &h
}

const (
	keyLen   = 32
	nonceLen = 24
)

func newNonce(r io.Reader) (*[nonceLen]byte, error) {
	var nonce [nonceLen]byte
	_, err := r.Read(nonce[:])
	if err != nil {
		return nil, fmt.Errorf("cannot generate random bytes: %v", err)
	}
	return &nonce, nil
}

func encrypt(key, text []byte, r io.Reader) ([]byte, error) {
	nonce, err := newNonce(r)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(nonce)+secretbox.Overhead+len(text))
	out = append(out, nonce[:]...)
	return secretbox.Seal(out, text, nonce, makeKey(key)), nil
}

func decrypt(key, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < nonceLen+secretbox.Overhead {
		return nil, fmt.Errorf("message too short")
	}
	var nonce [nonceLen]byte
	copy(nonce[:], ciphertext)
	ciphertext = ciphertext[nonceLen:]
	text, ok := secretbox.Open(nil, ciphertext, &nonce, makeKey(key))
	if !ok {
		return nil, fmt.Errorf("decryption failure")
	}
	return text, nil
}
