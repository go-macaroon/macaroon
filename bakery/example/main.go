// This example demonstrates three components:
//
// - A target service, representing a web server that
// wishes to use macaroons for authorization.
// It delegates authorization to a third-party
// authorization server by adding third-party
// caveats to macaroons that it sends to the user.
//
// - A client, representing a client wanting to make
// requests to the server.
//
// - An authorization server.
//
// In a real system, these three components would
// live on different machines; the client component
// could also be a web browser.
// (TODO: write javascript discharge gatherer)
package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
)

func main() {
	authEndpoint := mustServe(authService)
	serverEndpoint := mustServe(func(endpoint string) (http.Handler, error) {
		return targetService(endpoint, authEndpoint)
	})
	resp, err := clientRequest(serverEndpoint)
	if err != nil {
		log.Fatalf("client failed: %v", err)
	}
	fmt.Printf("client success: %q\n", resp)
}

func mustServe(newHandler func(string) (http.Handler, error)) (endpointURL string) {
	endpoint, err := serve(newHandler)
	if err != nil {
		log.Fatalf("cannot serve: %v", err)
	}
	return endpoint
}

func serve(newHandler func(string) (http.Handler, error)) (endpointURL string, err error) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return "", fmt.Errorf("cannot listen: %v", err)
	}
	endpointURL = "http://" + listener.Addr().String()
	handler, err := newHandler(endpointURL)
	if err != nil {
		return "", fmt.Errorf("cannot start handler: %v", err)
	}
	go http.Serve(listener, handler)
	return endpointURL, nil
}
