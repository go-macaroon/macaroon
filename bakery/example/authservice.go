package main

import (
	"net/http"

	"github.com/rogpeppe/macaroon/bakery"
	"github.com/rogpeppe/macaroon/httpbakery"
)

// Authorization service.
// This service can act as a checker for third party caveats.

func authService(endpoint string) (http.Handler, error) {
	svc, err := httpbakery.NewService(httpbakery.NewServiceParams{
		Location: endpoint,
	})
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	svc.AddDischargeHandler("/", mux, thirdPartyChecker)
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
