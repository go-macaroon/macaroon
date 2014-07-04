package main

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/rogpeppe/macaroon/httpbakery"
)

// client represents a client of the target service.
// In this simple example, it just tries a GET
// request, which will fail unless the client
// has the required authorization.
func clientRequest(serverEndpoint string) (string, error) {
	req, err := http.NewRequest("GET", serverEndpoint, nil)
	if err != nil {
		return "", fmt.Errorf("new request error: %v", err)
	}
	// The Do function implements the mechanics
	// of actually gathering discharge macaroons
	// when required, and retrying the request
	// when necessary.
	resp, err := httpbakery.Do(http.DefaultClient, req)
	if err != nil {
		return "", fmt.Errorf("GET failed: %v", err)
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("cannot read response: %v", err)
	}
	return string(data), nil
}
