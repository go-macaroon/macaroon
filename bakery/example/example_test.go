package main

import (
	"net/http"
	"testing"

	gc "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type exampleSuite struct{}

var _ = gc.Suite(&exampleSuite{})

func (*exampleSuite) TestExample(c *gc.C) {
	authEndpoint, err := serve(authService)
	c.Assert(err, gc.IsNil)
	serverEndpoint, err := serve(func(endpoint string) (http.Handler, error) {
		return targetService(endpoint, authEndpoint)
	})
	c.Assert(err, gc.IsNil)

	c.Logf("gold request")
	resp, err := clientRequest(serverEndpoint + "/gold")
	c.Assert(err, gc.IsNil)
	c.Assert(resp, gc.Equals, "all is golden")

	c.Logf("silver request")
	resp, err = clientRequest(serverEndpoint + "/silver")
	c.Assert(err, gc.IsNil)
	c.Assert(resp, gc.Equals, "every cloud has a silver lining")
}

func (*exampleSuite) BenchmarkExample(c *gc.C) {
	authEndpoint, err := serve(authService)
	c.Assert(err, gc.IsNil)
	serverEndpoint, err := serve(func(endpoint string) (http.Handler, error) {
		return targetService(endpoint, authEndpoint)
	})
	c.Assert(err, gc.IsNil)
	c.ResetTimer()
	for i := 0; i < c.N; i++ {
		resp, err := clientRequest(serverEndpoint)
		c.Assert(err, gc.IsNil)
		c.Assert(resp, gc.Equals, "hello, world\n")
	}
}
