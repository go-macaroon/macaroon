// The bakery package layers on top of the macaroon package, providing
// a transport and storage-agnostic way of using macaroons to assert
// client capabilities.
//
//
package bakery

import (
	"crypto/rand"
	"fmt"
	"log"
	"sync"

	"github.com/rogpeppe/macaroon"
)

// Service represents a service which can use macaroons
// to check authorization.
type Service struct {
	location        string
	store           storage
	checker         FirstPartyChecker
	caveatIdEncoder CaveatIdEncoder
}

// NewServiceParams holds the parameters for a NewService call.
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

// NewService returns a new service that can mint new
// macaroons and store their associated root keys.
func NewService(p NewServiceParams) *Service {
	if p.Store == nil {
		p.Store = NewMemStorage()
	}
	return &Service{
		location:        p.Location,
		store:           storage{p.Store},
		caveatIdEncoder: p.CaveatIdEncoder,
	}
}

// Store returns the store used by the service.
func (svc *Service) Store() Storage {
	return svc.store.store
}

// CaveatIdDecoder decodes caveat ids created by a CaveatIdEncoder.
type CaveatIdDecoder interface {
	DecodeCaveatId(id string) (rootKey []byte, condition string, err error)
}

// CaveatIdEncoder can create caveat ids for
// third parties. It is left abstract to allow location-dependent
// caveat id creation.
type CaveatIdEncoder interface {
	EncodeCaveatId(caveat Caveat, rootKey []byte) (string, error)
}

// Caveat represents a condition that must be true for a check to
// complete successfully. If Location is non-empty, the caveat must be
// discharged by a third party at the given location.
type Caveat struct {
	Location  string
	Condition string
}

// Request represents a request made to a service
// by a client. The request may be long-lived. It holds a set
// of macaroons that the client wishes to be taken
// into account.
//
// Methods on a Request may be called concurrently
// with each other.
type Request struct {
	svc     *Service
	checker FirstPartyChecker

	// mu guards the fields following it.
	mu sync.Mutex

	// macaroons holds the set of macaroons currently associated
	// with the request.
	macaroons []*macaroon.Macaroon

	// inStorage maps from macaroon id
	// to the storage associated with that macaroon
	// for all elements in macaroons.
	inStorage map[*macaroon.Macaroon]*storageItem

	// capability maps from a capability id to the macaroons
	// in the request that may discharge that capability.
	capability map[string][]*macaroon.Macaroon
}

// NewRequest returns a new client request object that uses checker to
// verify caveats.
func (svc *Service) NewRequest(checker FirstPartyChecker) *Request {
	return &Request{
		svc:        svc,
		checker:    checker,
		inStorage:  make(map[*macaroon.Macaroon]*storageItem),
		capability: make(map[string][]*macaroon.Macaroon),
	}
}

// AddClientMacaroon associates the given macaroon  with
// the request. The macaroon will be taken into account when req.Check
// is called.
//
// TODO(rog) provide a way of deleting client macaroons?
func (req *Request) AddClientMacaroon(m *macaroon.Macaroon) {
	req.mu.Lock()
	defer req.mu.Unlock()

	req.macaroons = append(req.macaroons, m)
	if req.inStorage[m] != nil {
		return
	}
	// TODO(rog) perhaps defer doing this until Check time,
	// when we could fetch all the ids at once. We'd
	// want to change Storage.Get to take a slice of ids.
	item, err := req.svc.store.Get(m.Id())
	if err == ErrNotFound {
		return
	}
	if err != nil {
		log.Printf("warning: failed to read storage: %v", err)
		return
	}
	req.inStorage[m] = item
	req.capability[item.Capability] = append(req.capability[item.Capability], m)
}

// NewMacaroon implements NewMacarooner.NewMacaroon.
func (svc *Service) NewMacaroon(id string, rootKey []byte, capability string, caveats []Caveat) (*macaroon.Macaroon, error) {
	if rootKey == nil {
		newRootKey, err := randomBytes(24)
		if err != nil {
			return nil, fmt.Errorf("cannot generate root key for new macaroon: %v", err)
		}
		rootKey = newRootKey
	}
	if id == "" {
		idBytes, err := randomBytes(24)
		if err != nil {
			return nil, fmt.Errorf("cannot generate id for new macaroon: %v", err)
		}
		id = fmt.Sprintf("%x", idBytes)
	}
	m := macaroon.New(rootKey, id, svc.location)

	// TODO look at the caveats for expiry time and associate
	// that with the storage item so that the storage can
	// garbage collect it at an appropriate time.
	if err := svc.store.Put(m.Id(), &storageItem{
		Capability: capability,
		RootKey:    rootKey,
	}); err != nil {
		return nil, fmt.Errorf("cannot save macaroon to store: %v", err)
	}
	for _, cav := range caveats {
		if err := svc.AddCaveat(m, cav); err != nil {
			if err := svc.store.store.Del(m.Id()); err != nil {
				log.Printf("failed to remove macaroon from storage: %v", err)
			}
			return nil, err
		}
	}
	return m, nil
}

// AddCaveat adds a caveat to the given macaroon.
//
// If it's a third-party caveat, it uses the service's caveat-id encoder
// to create the id of the new caveat.
func (svc *Service) AddCaveat(m *macaroon.Macaroon, cav Caveat) error {
	log.Printf("Service.AddCaveat id %q; cav %#v", m.Id(), cav)
	if cav.Location == "" {
		m.AddFirstPartyCaveat(cav.Condition)
		return nil
	}
	rootKey, err := randomBytes(24)
	if err != nil {
		return fmt.Errorf("cannot generate third party secret: %v", err)
	}
	id, err := svc.caveatIdEncoder.EncodeCaveatId(cav, rootKey)
	if err != nil {
		return fmt.Errorf("cannot create third party caveat id at %q: %v", cav.Location, err)
	}
	if err := m.AddThirdPartyCaveat(rootKey, id, cav.Location); err != nil {
		return fmt.Errorf("cannot add third party caveat: %v", err)
	}
	return nil
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, fmt.Errorf("cannot generate %d random bytes: %v", n, err)
	}
	return b, nil
}

// Check checks that the client has the given capability.
// If the verification fails in a way which might be remediable,
// it returns a VerificationError that describes the error.
//
// A capability represents a client capability. A client
// can gain a capability by presenting a valid, fully
// discharged macaroon that is associated with
// the capability.
func (req *Request) Check(capability string) error {
	// TODO(rog) provide a more flexible capability
	// system - perhaps delegate to an interface,
	// say:
	//
	// type Capability interface {
	//	IsCapable(requireCap, hasCap string) bool
	// }
	//
	// Then attempt to verify any macaroons which
	// for which IsCapable(m.capability, capability) is true.
	req.mu.Lock()
	defer req.mu.Unlock()
	possibleMacaroons := req.capability[capability]
	if len(possibleMacaroons) == 0 {
		// no macaroons discharging the capability.
		return &VerificationError{
			RequiredCapability: capability,
			Reason:             fmt.Errorf("no possible macaroons found"),
		}
	}
	var anError error
	for _, m := range possibleMacaroons {
		item := req.inStorage[m]
		err := m.Verify(item.RootKey, req.checker.CheckFirstPartyCaveat, req.macaroons)
		if err == nil {
			return nil
		}
		anError = err
	}
	return &VerificationError{
		RequiredCapability: capability,
		Reason:             anError,
	}
}

type CaveatNotRecognizedError struct {
	Caveat string
}

func (e *CaveatNotRecognizedError) Error() string {
	return fmt.Sprintf("caveat %q not recognized", e.Caveat)
}

type VerificationError struct {
	RequiredCapability string
	Reason             error
}

func (e *VerificationError) Error() string {
	return fmt.Sprintf("verification failed: %v", e.Reason)
}

// TODO(rog) consider possible options for checkers:
// - first and third party checkers could be merged, but
// then there would have to be a runtime check
// that when used to check first-party caveats, the
// checker does not return third-party caveats.

// ThirdPartyChecker holds a function that checks
// third party caveats for validity. It the
// caveat is valid, it returns a nil error and
// optionally a slice of extra caveats that
// will be added to the discharge macaroon.
//
// If the caveat kind was not recognised, the checker
// should return ErrCaveatNotRecognised.
type ThirdPartyChecker interface {
	CheckThirdPartyCaveat(caveat string) ([]Caveat, error)
}

type ThirdPartyCheckerFunc func(caveat string) ([]Caveat, error)

func (c ThirdPartyCheckerFunc) CheckThirdPartyCaveat(caveat string) ([]Caveat, error) {
	return c(caveat)
}

// FirstPartyChecker holds a function that checks
// first party caveats for validity.
//
// If the caveat kind was not recognised, the checker
// should return ErrCaveatNotRecognised.
type FirstPartyChecker interface {
	CheckFirstPartyCaveat(caveat string) error
}

type FirstPartyCheckerFunc func(caveat string) error

func (c FirstPartyCheckerFunc) CheckFirstPartyCaveat(caveat string) error {
	return c(caveat)
}
