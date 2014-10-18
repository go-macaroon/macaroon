package main

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/juju/errgo"

	"github.com/rogpeppe/macaroon/httpbakery"
)

// client represents a client of the target service.
// In this simple example, it just tries a GET
// request, which will fail unless the client
// has the required authorization.
func clientRequest(serverEndpoint string) (string, error) {
	req, err := http.NewRequest("GET", serverEndpoint, nil)
	if err != nil {
		return "", errgo.Notef(err, "cannot make new HTTP request")
	}
	// The Do function implements the mechanics
	// of actually gathering discharge macaroons
	// when required, and retrying the request
	// when necessary.

	visitWebPage := func(url string) error {
		fmt.Printf("please visit this web page:\n")
		fmt.Printf("\t%s\n", url)
		return nil
	}
	resp, err := httpbakery.Do(httpbakery.DefaultHTTPClient, req, visitWebPage)
	if err != nil {
		return "", errgo.NoteMask(err, "GET failed", errgo.Any)
	}
	defer resp.Body.Close()
	// TODO(rog) unmarshal error
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("cannot read response: %v", err)
	}
	return string(data), nil
}
