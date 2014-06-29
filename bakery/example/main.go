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
	"log"
	"net"
	"net/http"
)

func main() {
	authEndpoint := serve(authService)
	serverEndpoint := serve(func(endpoint string) (http.Handler, error) {
		return targetService(endpoint, authEndpoint)
	})
	client(serverEndpoint)
}

func serve(newHandler func(string) (http.Handler, error)) (endpointURL string) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Fatal(err)
	}
	endpointURL = "http://" + listener.Addr().String()
	handler, err := newHandler(endpointURL)
	if err != nil {
		log.Fatal(err)
	}
	go http.Serve(listener, handler)
	return endpointURL
}
