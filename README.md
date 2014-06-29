# macaroon
--
    import "github.com/rogpeppe/macaroon"

The macaroon package implements macaroons as described in the paper "Macaroons:
Cookies with Contextual Caveats for Decentralized Authorization in the Cloud"
(http://theory.stanford.edu/~ataly/Papers/macaroons.pdf)

## Usage

#### type Caveat

```go
type Caveat struct {
}
```

Caveat holds a first person or third party caveat.

#### func (*Caveat) Id

```go
func (cav *Caveat) Id() string
```

#### func (*Caveat) IsThirdParty

```go
func (cav *Caveat) IsThirdParty() bool
```
IsThirdParty reports whether the caveat must be satisfied by some third party
(if not, it's a first person caveat).

#### func (*Caveat) Location

```go
func (cav *Caveat) Location() string
```

#### func (*Caveat) MarshalJSON

```go
func (cav *Caveat) MarshalJSON() ([]byte, error)
```
MarshalJSON implements json.Marshaler.

#### func (*Caveat) UnmarshalJSON

```go
func (cav *Caveat) UnmarshalJSON(jsonData []byte) error
```
unmarshalJSON implements json.Unmarshaler.

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
func New(rootKey []byte, id, loc string) *Macaroon
```
New returns a new macaroon with the given root key, identifier and location.

#### func (*Macaroon) AddFirstPartyCaveat

```go
func (m *Macaroon) AddFirstPartyCaveat(caveatId string)
```
AddFirstPartyCaveat adds a caveat that will be verified by the target service.

#### func (*Macaroon) AddThirdPartyCaveat

```go
func (m *Macaroon) AddThirdPartyCaveat(rootKey []byte, caveatId string, loc string) error
```
AddThirdPartyCaveat adds a third-party caveat to the macaroon, using the given
shared root key, caveat id and location hint. The caveat id should encode the
root key in some way, either by encrypting it with a key known to the third
party or by holding a reference to it stored in the third party's storage.

#### func (*Macaroon) Bind

```go
func (m *Macaroon) Bind(rootSig []byte)
```
Bind prepares the macaroon for being used to discharge the macaroon with the
given rootSig. This must be used before it is used in the discharges argument to
Verify.

#### func (*Macaroon) Caveats

```go
func (m *Macaroon) Caveats() []Caveat
```
Caveats returns the macaroon's caveats. This method will probably change, and
it's important not to change the returned caveat.

#### func (*Macaroon) Clone

```go
func (m *Macaroon) Clone() *Macaroon
```
Clone returns a copy of the receiving macaroon.

#### func (*Macaroon) Id

```go
func (m *Macaroon) Id() string
```
Id returns the id of the macaroon. This can hold arbitrary information.

#### func (*Macaroon) Location

```go
func (m *Macaroon) Location() string
```
Location returns the macaroon's location hint. This is not verified as part of
the macaroon.

#### func (*Macaroon) MarshalJSON

```go
func (m *Macaroon) MarshalJSON() ([]byte, error)
```
MarshalJSON implements json.Marshaler.

#### func (*Macaroon) Signature

```go
func (m *Macaroon) Signature() []byte
```
Signature returns the macaroon's signature.

#### func (*Macaroon) UnmarshalJSON

```go
func (m *Macaroon) UnmarshalJSON(jsonData []byte) error
```
UnmarshalJSON implements json.Unmarshaler.

#### func (*Macaroon) Verify

```go
func (m *Macaroon) Verify(rootKey []byte, check func(caveat string) error, discharges []*Macaroon) error
```
Verify verifies that the receiving macaroon is valid. The root key must be the
same that the macaroon was originally minted with. The check function is called
to verify each first-party caveat - it should return an error if the condition
is not met.

The discharge macaroons should be provided in discharges.

Verify returns true if the verification succeeds; if returns (false, nil) if the
verification fails, and (false, err) if the verification cannot be asserted (but
may not be false).

TODO(rog) is there a possible DOS attack that can cause this function to
infinitely recurse?

#### type Verifier

```go
type Verifier interface {
	Verify(m *Macaroon, rootKey []byte) (bool, error)
}
```
# bakery
--
    import "github.com/rogpeppe/macaroon/bakery"

The bakery package layers on top of the macaroon package, providing a transport
and storage-agnostic way of using macaroons to assert client capabilities.

## Usage

```go
var ErrNotFound = errors.New("item not found")
```

#### type Caveat

```go
type Caveat struct {
	Location  string
	Condition string
}
```

Caveat represents a condition that must be true for a check to complete
successfully. If Location is non-empty, the caveat must be discharged by a third
party at the given location.

#### type CaveatIdDecoder

```go
type CaveatIdDecoder interface {
	DecodeCaveatId(id string) (rootKey []byte, condition string, err error)
}
```

CaveatIdDecoder decodes caveat ids created by a CaveatIdEncoder.

#### type CaveatIdEncoder

```go
type CaveatIdEncoder interface {
	EncodeCaveatId(caveat Caveat, rootKey []byte) (string, error)
}
```

CaveatIdEncoder can create caveat ids for third parties. It is left abstract to
allow location-dependent caveat id creation.

#### type CaveatNotRecognizedError

```go
type CaveatNotRecognizedError struct {
	Caveat string
}
```


#### func (*CaveatNotRecognizedError) Error

```go
func (e *CaveatNotRecognizedError) Error() string
```

#### type Discharger

```go
type Discharger struct {
	// Checker is used to check the caveat's condition.
	Checker ThirdPartyChecker

	// Decoder is used to decode the caveat id.
	Decoder CaveatIdDecoder

	// Factory is used to create the macaroon.
	// Note that *Service implements NewMacarooner.
	Factory NewMacarooner
}
```

A Discharger can be used to discharge third party caveats.

#### func (*Discharger) Discharge

```go
func (d *Discharger) Discharge(id string) (*macaroon.Macaroon, error)
```
Discharge creates a macaroon that discharges the third party caveat with the
given id. The id should have been created earlier with a matching
CaveatIdEncoder. The condition implicit in the id is checked for validity using
d.Checker, and then if valid, a new macaroon is minted which discharges the
caveat, and can eventually be associated with a client request using
AddClientMacaroon.

#### type FirstPartyChecker

```go
type FirstPartyChecker interface {
	CheckFirstPartyCaveat(caveat string) error
}
```

FirstPartyChecker holds a function that checks first party caveats for validity.

If the caveat kind was not recognised, the checker should return
ErrCaveatNotRecognised.

#### type FirstPartyCheckerFunc

```go
type FirstPartyCheckerFunc func(caveat string) error
```


#### func (FirstPartyCheckerFunc) CheckFirstPartyCaveat

```go
func (c FirstPartyCheckerFunc) CheckFirstPartyCaveat(caveat string) error
```

#### type NewMacarooner

```go
type NewMacarooner interface {
	NewMacaroon(id string, rootKey []byte, capability string, caveats []Caveat) (*macaroon.Macaroon, error)
}
```

NewMacaroon mints a new macaroon with the given id, capability and caveats. If
the id is empty, a random id will be used. If rootKey is nil, a random root key
will be used.

If a client succeeds in discharging the returned macaroon, it can gain access to
the given capability.

#### type NewServiceParams

```go
type NewServiceParams struct {
	// Location will be set as the location of any macaroons
	// minted by the service.
	Location string

	// Store will be used to store macaroon
	// information locally. If it is nil,
	// an in-memory storage will be used.
	Store Storage

	// CaveatIdEncoder is used to create third-party caveats.
	CaveatIdEncoder CaveatIdEncoder
}
```

NewServiceParams holds the parameters for a NewService call.

#### type Request

```go
type Request struct {
}
```

Request represents a request made to a service by a client. The request may be
long-lived. It holds a set of macaroons that the client wishes to be taken into
account.

Methods on a Request may be called concurrently with each other.

#### func (*Request) AddClientMacaroon

```go
func (req *Request) AddClientMacaroon(m *macaroon.Macaroon)
```
AddClientMacaroon associates the given macaroon with the request. The macaroon
will be taken into account when req.Check is called.

TODO(rog) provide a way of deleting client macaroons?

#### func (*Request) Check

```go
func (req *Request) Check(capability string) error
```
Check checks that the client has the given capability. If the verification fails
in a way which might be remediable, it returns a VerificationError that
describes the error.

A capability represents a client capability. A client can gain a capability by
presenting a valid, fully discharged macaroon that is associated with the
capability.

#### type Service

```go
type Service struct {
}
```

Service represents a service which can use macaroons to check authorization.

#### func  NewService

```go
func NewService(p NewServiceParams) *Service
```
NewService returns a new service that can mint new macaroons and store their
associated root keys.

#### func (*Service) AddCaveat

```go
func (svc *Service) AddCaveat(m *macaroon.Macaroon, cav Caveat) error
```
AddCaveat adds a caveat to the given macaroon.

If it's a third-party caveat, it uses the service's caveat-id encoder to create
the id of the new caveat.

#### func (*Service) NewMacaroon

```go
func (svc *Service) NewMacaroon(id string, rootKey []byte, capability string, caveats []Caveat) (*macaroon.Macaroon, error)
```
NewMacaroon implements NewMacarooner.NewMacaroon.

#### func (*Service) NewRequest

```go
func (svc *Service) NewRequest(checker FirstPartyChecker) *Request
```
NewRequest returns a new client request object that uses checker to verify
caveats.

#### func (*Service) Store

```go
func (svc *Service) Store() Storage
```
Store returns the store used by the service.

#### type Storage

```go
type Storage interface {
	// Put stores the item at the given location, overwriting
	// any item that might already be there.
	// TODO(rog) would it be better to lose the overwrite
	// semantics?
	Put(location string, item string) error

	// Get retrieves an item from the given location.
	// If the item is not there, it returns ErrNotFound.
	Get(location string) (item string, err error)

	// Del deletes the item from the given location.
	Del(location string) error
}
```

Storage defines storage for macaroons. Calling its methods concurrently is
allowed.

#### func  NewMemStorage

```go
func NewMemStorage() Storage
```
NewMemStorage returns an implementation of Storage that stores all items in
memory.

#### type ThirdPartyChecker

```go
type ThirdPartyChecker interface {
	CheckThirdPartyCaveat(caveat string) ([]Caveat, error)
}
```

ThirdPartyChecker holds a function that checks third party caveats for validity.
It the caveat is valid, it returns a nil error and optionally a slice of extra
caveats that will be added to the discharge macaroon.

If the caveat kind was not recognised, the checker should return
ErrCaveatNotRecognised.

#### type ThirdPartyCheckerFunc

```go
type ThirdPartyCheckerFunc func(caveat string) ([]Caveat, error)
```


#### func (ThirdPartyCheckerFunc) CheckThirdPartyCaveat

```go
func (c ThirdPartyCheckerFunc) CheckThirdPartyCaveat(caveat string) ([]Caveat, error)
```

#### type VerificationError

```go
type VerificationError struct {
	RequiredCapability string
	Reason             error
}
```


#### func (*VerificationError) Error

```go
func (e *VerificationError) Error() string
```
# checkers
--
    import "github.com/rogpeppe/macaroon/bakery/checkers"

The checkers package provides some standard caveat checkers and
checker-combining functions.

## Usage

```go
var Std = Map{
	"time-before": bakery.FirstPartyCheckerFunc(timeBefore),
}
```

#### func  FirstParty

```go
func FirstParty(condition string) bakery.Caveat
```

#### func  ParseCaveat

```go
func ParseCaveat(cav string) (string, string, error)
```
ParseCaveat parses a caveat into an identifier, identifying the checker that
should be used, and the argument to the checker (the rest of the string).

The identifier is taken from all the characters before the first space
character.

#### func  PushFirstPartyChecker

```go
func PushFirstPartyChecker(c0, c1 bakery.FirstPartyChecker) bakery.FirstPartyChecker
```
PushFirstPartyChecker returns a checker that first uses c0 to check caveats, and
falls back to using c1 if c0 returns bakery.ErrCaveatNotRecognized.

#### func  ThirdParty

```go
func ThirdParty(location, condition string) bakery.Caveat
```

#### func  TimeBefore

```go
func TimeBefore(t time.Time) bakery.Caveat
```

#### type Map

```go
type Map map[string]bakery.FirstPartyCheckerFunc
```


#### func (Map) CheckFirstPartyCaveat

```go
func (m Map) CheckFirstPartyCaveat(cav string) error
```
# example
--
This example demonstrates three components:

- A target service, representing a web server that wishes to use macaroons for
authorization. It delegates authorization to a third-party authorization server
by adding third-party caveats to macaroons that it sends to the user.

- A client, representing a client wanting to make requests to the server.

- An authorization server.

In a real system, these three components would live on different machines; the
client component could also be a web browser. (TODO: write javascript discharge
gatherer)
# httpbakery
--
    import "github.com/rogpeppe/macaroon/httpbakery"

The httpbakery package layers on top of the bakery package - it provides an
HTTP-based implementation of a macaroon client and server.

## Usage

#### func  Do

```go
func Do(c *http.Client, req *http.Request) (*http.Response, error)
```
Do makes an http request to the given client. If the request fails with a
discharge-required error, any required discharge macaroons will be acquired, and
the request will be repeated with those attached.

If c.Jar field is non-nil, the macaroons will be stored there and made available
to subsequent requests.

#### func  WriteDischargeRequiredError

```go
func WriteDischargeRequiredError(w http.ResponseWriter, m *macaroon.Macaroon, originalErr error) error
```
WriteDischargeRequiredError writes a response to w that reports the given error
and sends the given macaroon to the client, indicating that it should be
discharged to allow the original request to be accepted.

If it returns an error, it will have written the http response anyway.

The cookie value is a base-64-encoded JSON serialization of the macaroon.

TODO(rog) consider an alternative approach - perhaps it would be better to
include the macaroon directly in the response and leave it up to the client to
add it to the cookies along with the discharge macaroons.

#### type KeyPair

```go
type KeyPair struct {
}
```

KeyPair holds a public/private pair of keys.

#### func  GenerateKey

```go
func GenerateKey() (*KeyPair, error)
```
GenerateKey generates a new key pair.

#### type NewServiceParams

```go
type NewServiceParams struct {
	// Location holds the location of the service.
	// Macaroons minted by the service will have this location.
	Location string

	// Store defines where macaroons are stored.
	Store bakery.Storage

	// Key holds the private/public key pair for
	// the service to use. If it is nil, a new key pair
	// will be generated.
	Key *KeyPair
}
```

NewServiceParams holds parameters for the NewService call.

#### type Service

```go
type Service struct {
	*bakery.Service
}
```

Service represents a service that can use client-provided macaroons to authorize
requests. It layers on top of *bakery.Service, providing http-based methods to
create third-party caveats.

#### func  NewService

```go
func NewService(p NewServiceParams) (*Service, error)
```
NewService returns a new Service.

#### func (*Service) AddDischargeHandler

```go
func (svc *Service) AddDischargeHandler(
	root string,
	mux *http.ServeMux,
	checker func(req *http.Request, cav string) ([]bakery.Caveat, error),
)
```
AddDischargeHandler handles adds handlers to the given ServeMux to service third
party caveats.

The check function is used to check whether a client making the given request
should be allowed a discharge for the given caveat. If it does not return an
error, the caveat will be discharged, with any returned caveats also added to
the discharge macaroon.

The name space served by DischargeHandler is as follows. All parameters can be
provided either as URL attributes or form attributes. The result is always
formatted as a JSON object.

POST /discharge

    params:
    	id: id of macaroon to discharge
    	location: location of original macaroon (optional (?))
    result:
    	{
    		Macaroon: macaroon in json format
    		Error: string
    	}

POST /create

    params:
    	condition: caveat condition to discharge
    	rootkey: root key of discharge caveat
    result:
    	{
    		CaveatID: string
    		Error: string
    	}

GET /publickey

    result:
    	public key of service
    	expiry time of key

#### func (*Service) AddPublicKeyForLocation

```go
func (svc *Service) AddPublicKeyForLocation(loc string, prefix bool, publicKey *[32]byte)
```
AddPublicKeyForLocation specifies that third party caveats for the given
location will be encrypted with the given public key. If prefix is true, any
locations with loc as a prefix will be also associated with the given key. The
longest prefix match will be chosen. TODO(rog) perhaps string might be a better
representation of public keys?

#### func (*Service) NewRequest

```go
func (svc *Service) NewRequest(httpReq *http.Request, checker bakery.FirstPartyChecker) *bakery.Request
```
NewRequest returns a new request, converting cookies from the HTTP request into
macaroons in the bakery request when they're found. Mmm.
