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
	listener, err := net.Listen("tcp", ":0")
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
