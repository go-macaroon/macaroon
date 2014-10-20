// The httpbakery package layers on top of the bakery
// package - it provides an HTTP-based implementation
// of a macaroon client and server.
package httpbakery

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"

	"code.google.com/p/go.net/publicsuffix"

	"github.com/rogpeppe/macaroon"
	"github.com/rogpeppe/macaroon/bakery"
)

// Service represents a service that can use client-provided
// macaroons to authorize requests. It layers on top
// of *bakery.Service, providing http-based methods
// to create third-party caveats.
type Service struct {
	*bakery.Service
	caveatIdEncoder *caveatIdEncoder
	key             KeyPair
}

// Key returns the service's private/public key pair.
func (svc *Service) Key() *KeyPair {
	return &svc.key
}

// DefaultHTTPClient is an http.Client that ensures that
// headers are sent to the server even when the server redirects.
var DefaultHTTPClient = defaultHTTPClient()

func defaultHTTPClient() *http.Client {
	c := *http.DefaultClient
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		if len(via) == 0 {
			return nil
		}
		for attr, val := range via[0].Header {
			if _, ok := req.Header[attr]; !ok {
				req.Header[attr] = val
			}
		}
		return nil
	}
	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})
	if err != nil {
		panic(err)
	}
	c.Jar = &cookieLogger{jar}
	return &c
}

type cookieLogger struct {
	http.CookieJar
}

func (j *cookieLogger) SetCookies(u *url.URL, cookies []*http.Cookie) {
	log.Printf("%p setting %d cookies for %s", j.CookieJar, len(cookies), u)	
	for i, c := range cookies {
		log.Printf("\t%d. path %s; name %s", i, c.Path, c.Name)
	}
	j.CookieJar.SetCookies(u, cookies)
}

// NewServiceParams holds parameters for the NewService call.
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

	// HTTPClient holds the http client to use when
	// creating new third party caveats for third
	// parties. If it is nil, DefaultHTTPClient will be used.
	HTTPClient *http.Client
}

// NewService returns a new Service.
func NewService(p NewServiceParams) (*Service, error) {
	if p.Key == nil {
		key, err := GenerateKey()
		if err != nil {
			return nil, fmt.Errorf("cannot generate key: %v", err)
		}
		p.Key = key
	}
	log.Printf("new service at %s with public key %x", p.Location, p.Key.public[:])
	if p.HTTPClient == nil {
		p.HTTPClient = DefaultHTTPClient
	}
	enc := newCaveatIdEncoder(p.HTTPClient, p.Key)
	return &Service{
		Service: bakery.NewService(bakery.NewServiceParams{
			Location:        p.Location,
			Store:           p.Store,
			CaveatIdEncoder: enc,
		}),
		caveatIdEncoder: enc,
		key:             *p.Key,
	}, nil
}

// AddPublicKeyForLocation specifies that third party caveats
// for the given location will be encrypted with the given public
// key. If prefix is true, any locations with loc as a prefix will
// be also associated with the given key. The longest prefix
// match will be chosen.
// TODO(rog) perhaps string might be a better representation
// of public keys?
// TODO(rog) strict string prefix is bad when locations
// are URLs. We should probably parse them as URLs
// and dispatch in a more intelligent way (for example
// by matching host name exactly and the path by
// full path name elements only.)
func (svc *Service) AddPublicKeyForLocation(loc string, prefix bool, publicKey *[32]byte) {
	svc.caveatIdEncoder.addPublicKeyForLocation(loc, prefix, publicKey)
}

// Discharger returns a discharger that uses the receiving service
// to create its macaroons and to decode third-party caveat ids.
// The decoded caveat ids are checked using the provided
// checker.
func (svc *Service) Discharger(checker bakery.ThirdPartyChecker) *bakery.Discharger {
	return &bakery.Discharger{
		Checker: checker,
		Decoder: newCaveatIdDecoder(svc.Store(), svc.Key()),
		Factory: svc,
	}
}

// NewRequest returns a new request, converting cookies from the
// HTTP request into macaroons in the bakery request when they're
// found. Mmm.
func (svc *Service) NewRequest(httpReq *http.Request, checker bakery.FirstPartyChecker) *bakery.Request {
	req := svc.Service.NewRequest(checker)
	for _, cookie := range httpReq.Cookies() {
		if !strings.HasPrefix(cookie.Name, "macaroon-") {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(cookie.Value)
		if err != nil {
			log.Printf("cannot base64-decode cookie; ignoring: %v", err)
			continue
		}
		var m macaroon.Macaroon
		if err := m.UnmarshalJSON(data); err != nil {
			log.Printf("cannot unmarshal macaroon from cookie; ignoring: %v", err)
			continue
		}
		req.AddClientMacaroon(&m)
	}
	return req
}
