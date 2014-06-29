package main

import (
	"io"
	"log"
	"net/http"
	"os"

	"github.com/rogpeppe/macaroon/httpbakery"
)

// client represents a client of the target service.
// In this simple example, it just tries a GET
// request, which will fail unless the client
// has the required authorization.
func client(serverEndpoint string) {
	req, err := http.NewRequest("GET", serverEndpoint, nil)
	if err != nil {
		log.Fatalf("new request error: %v", err)
	}
	// The Do function implements the mechanics
	// of actually gathering discharge macaroons
	// when required, and retrying the request
	// when necessary.
	resp, err := httpbakery.Do(http.DefaultClient, req)
	if err != nil {
		log.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()
	log.Printf("GET %s succeeded. status %s", serverEndpoint, resp.Status)
	io.Copy(os.Stdout, resp.Body)
}
