package main

import (
	"fmt"
	"net/http"

	"github.com/rogpeppe/macaroon/bakery"
	"github.com/rogpeppe/macaroon/httpbakery"
)

// Authorization service.
// This service can act as a checker for third party caveats.

func authService(endpoint string) (http.Handler, error) {
	enc, err := httpbakery.NewCaveatIdEncoder(nil)
	if err != nil {
		return nil, fmt.Errorf("cannot create caveat id decoder: %v", err)
	}
	svc := bakery.NewService(bakery.NewServiceParams{
		Location:        endpoint,
		CaveatIdEncoder: enc,
	})
	mux := http.NewServeMux()
	httpbakery.AddDischargeHandler("/", mux, svc, nil, thirdPartyChecker)
	return mux, nil
}

func thirdPartyChecker(req *http.Request, condition string) ([]bakery.Caveat, error) {
	if condition != "access-allowed" {
		return nil, bakery.ErrCaveatNotRecognized
	}
	// TODO check that the HTTP request has cookies that prove
	// something about the client.
	return []bakery.Caveat{{
		Condition: "peer-is localhost",
	}}, nil
}
