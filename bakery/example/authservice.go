package main

import (
	"net/http"

	"github.com/rogpeppe/macaroon/bakery"
	"github.com/rogpeppe/macaroon/httpbakery"
)

// authService implements an authorization service,
// that can discharge third-party caveats added
// to other macaroons.
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

// thirdPartyChecker is used to check third party caveats added by other
// services. The HTTP request is that of the client - it is attempting
// to gather a discharge macaroon.
//
// Note how this function can return additional first- and third-party
// caveats which will be added to the original macaroon's caveats.
func thirdPartyChecker(req *http.Request, condition string) ([]bakery.Caveat, error) {
	if condition != "access-allowed" {
		return nil, &bakery.CaveatNotRecognizedError{condition}
	}
	// TODO check that the HTTP request has cookies that prove
	// something about the client.
	return []bakery.Caveat{{
		Condition: "remote-host 127.0.0.1",
	}}, nil
}
