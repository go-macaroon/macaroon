# macaroon
--
    import "github.com/rogpeppe/macaroon"

The macaroon package implements macaroons as described in the paper "Macaroons:
Cookies with Contextual Caveats for Decentralized Authorization in the Cloud"
(http://theory.stanford.edu/~ataly/Papers/macaroons.pdf)

It still in its very early stages, having no support for serialisation and only
rudimentary test coverage.

## Usage

#### type Caveat

```go
type Caveat struct {
}
```

Caveat holds a first person or third party caveat.

#### func (*Caveat) IsThirdParty

```go
func (cav *Caveat) IsThirdParty() bool
```
IsThirdParty reports whether the caveat must be satisfied by some third party
(if not, it's a first person caveat).

#### type Macaroon

```go
type Macaroon struct {
}
```

Macaroon holds a macaroon. See Fig. 7 of
http://theory.stanford.edu/~ataly/Papers/macaroons.pdf for a description of the
data contained within. Macaroons are mutable objects - use Clone as appropriate
to avoid unwanted mutation.

#### func  New

```go
func New(rootKey, id []byte, loc string) *Macaroon
```
New returns a new macaroon with the given root key, identifier and location.

#### func (*Macaroon) AddFirstPartyCaveat

```go
func (m *Macaroon) AddFirstPartyCaveat(caveat string)
```
AddFirstPartyCaveat adds a caveat that will be verified by the target service.

#### func (*Macaroon) AddThirdPartyCaveat

```go
func (m *Macaroon) AddThirdPartyCaveat(thirdPartySecret []byte, caveat string, loc string) (id []byte, err error)
```
AddThirdPartyCaveat adds a third-party caveat to the macaroon, using the given
shared secret, caveat and location hint. It returns the caveat id of the third
party macaroon.

#### func (*Macaroon) Bind

```go
func (m *Macaroon) Bind(rootSig []byte)
```
Bind prepares the macaroon for being used to discharge the macaroon with the
given rootSig. This must be used before it is used in the discharges argument to
Verify.

#### func (*Macaroon) Clone

```go
func (m *Macaroon) Clone() *Macaroon
```
Clone returns a copy of the receiving macaroon.

#### func (*Macaroon) Id

```go
func (m *Macaroon) Id() []byte
```
Id returns the id of the macaroon. This can hold arbitrary information.

#### func (*Macaroon) Location

```go
func (m *Macaroon) Location() string
```
Location returns the macaroon's location hint. This is not verified as part of
the macaroon.

#### func (*Macaroon) Signature

```go
func (m *Macaroon) Signature() []byte
```
Signature returns the macaroon's signature.

#### func (*Macaroon) Verify

```go
func (m *Macaroon) Verify(rootKey []byte, check func(caveat string) (bool, error), discharges map[string]*Macaroon) (bool, error)
```
Verify verifies that the receiving macaroon is valid. The root key must be the
same that the macaroon was originally minted with. The check function is called
to verify each first-party caveat - it may return an error the check cannot be
made but the answer is not necessarily false. The discharge macaroons should be
passed in discharges, keyed by macaroon id.

Verify returns true if the verification succeeds; if returns (false, nil) if the
verification fails, and (false, err) if the verification cannot be asserted (but
may not be false).

#### type ThirdPartyCaveatId

```go
type ThirdPartyCaveatId struct {
	RootKey []byte
	Caveat  string
}
```

ThirdPartyCaveatId holds the information encoded in a third-party caveat id.

#### func  DecryptThirdPartyCaveatId

```go
func DecryptThirdPartyCaveatId(secret, id []byte) (*ThirdPartyCaveatId, error)
```
DecryptThirdPartyCaveatId decrypts a third-party caveat id given the shared
secret.

#### type Verifier

```go
type Verifier interface {
	Verify(m *Macaroon, rootKey []byte) (bool, error)
}
```
