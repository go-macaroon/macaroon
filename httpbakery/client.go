package httpbakery

import (
	"net/http"
)

// Do makes an http request to the given client.
// If the request fails with a discharge-required error,
// any required discharge macaroons will be acquired,
// and the request will be repeated with those attached.
//
// If c.Jar field is non-nil, the macaroons will be
// stored there and made available to subsequent requests.
func Do(c *http.Client, req *http.Request) (*http.Response, error) {
	panic("unimplemented")
}
