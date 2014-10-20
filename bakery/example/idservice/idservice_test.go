package idservice_test

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/juju/errgo"
	gc "gopkg.in/check.v1"

	"github.com/rogpeppe/macaroon/bakery"
	"github.com/rogpeppe/macaroon/bakery/example/idservice"
	"github.com/rogpeppe/macaroon/httpbakery"
)

type suite struct {
	authEndpoint  string
	authPublicKey *[32]byte
}

var _ = gc.Suite(&suite{})

func (s *suite) SetUpSuite(c *gc.C) {
	key, err := httpbakery.GenerateKey()
	c.Assert(err, gc.IsNil)
	s.authPublicKey = key.PublicKey()
	s.authEndpoint = serve(c, func(endpoint string) (http.Handler, error) {
		return idservice.New(idservice.Params{
			Users: map[string]*idservice.UserInfo{
				"rog": {
					Password: "password",
				},
				"root": {
					Password: "superman",
					Groups: map[string]bool{
						"target-service-users": true,
					},
				},
			},
			Service: httpbakery.NewServiceParams{
				Location: endpoint,
				Store:    bakery.NewMemStorage(),
				Key:      key,
			},
		})
	})
	c.Logf("auth endpoint at %s", s.authEndpoint)
}

func (s *suite) TestIdService(c *gc.C) {
	serverEndpoint := serve(c, func(endpoint string) (http.Handler, error) {
		return targetService(endpoint, s.authEndpoint, s.authPublicKey)
	})
	c.Logf("target service endpoint at %s", serverEndpoint)
	visitDone := make(chan struct{})
	visitWebPage := func(u *url.URL) error {
		go func() {
			err := scrapeLoginPage(u)
			c.Logf("scrape returned %v", err)
			c.Check(err, gc.IsNil)
			visitDone <- struct{}{}
		}()
		return nil
	}
	resp, err := clientRequest(serverEndpoint+"/gold", visitWebPage)
	c.Assert(err, gc.IsNil)
	c.Assert(resp, gc.Equals, "all is golden")
	select {
	case <-visitDone:
	case <-time.After(5 * time.Second):
		c.Fatalf("visit never done")
	}

	// Try again. We shouldn't need to interact this time.
	resp, err = clientRequest(serverEndpoint+"/silver", noVisit)
	c.Assert(err, gc.IsNil)
	c.Assert(resp, gc.Equals, "every cloud has a silver lining")
}

func noVisit(*url.URL) error {
	return errgo.New("should not be visiting")
}

func serve(c *gc.C, newHandler func(string) (http.Handler, error)) (endpointURL string) {
	listener, err := net.Listen("tcp", "localhost:0")
	c.Assert(err, gc.IsNil)

	endpointURL = "http://" + listener.Addr().String()
	handler, err := newHandler(endpointURL)
	c.Assert(err, gc.IsNil)

	go http.Serve(listener, handler)
	return endpointURL
}

// client represents a client of the target service. In this simple
// example, it just tries a GET request, which will fail unless the
// client has the required authorization.
func clientRequest(serverEndpoint string, visitWebPage func(*url.URL) error) (string, error) {
	req, err := http.NewRequest("GET", serverEndpoint, nil)
	if err != nil {
		return "", errgo.Notef(err, "cannot make new HTTP request")
	}

	// The Do function implements the mechanics
	// of actually gathering discharge macaroons
	// when required, and retrying the request
	// when necessary.
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

// Patterns to search for the relevant information in the login page.
// Alternatives to this might be (in likely ascending order of complexity):
// - use the template itself as the pattern.
// - parse the html with encoding/xml
// - parse the html with code.google.com/p/go.net/html
var (
	actionPat = regexp.MustCompile(`<form action="([^"]+)"`)
	waitIdPat = regexp.MustCompile(`name="waitid" value="([^"]+)"`)
)

// scrapeLoginPage simulates a user visiting the given web
// page. It gets the login page, then does a POST with
// the appropriate form parameters.
func scrapeLoginPage(loginURL *url.URL) error {
	log.Printf("scraping login page")
	// Get the page.
	client := httpbakery.DefaultHTTPClient
	log.Printf("scrape: getting %s", loginURL)
	resp, err := client.Get(loginURL.String())
	if err != nil {
		return errgo.Mask(err)
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errgo.Notef(err, "cannot read body")
	}
	m := actionPat.FindSubmatch(data)
	if m == nil {
		return errgo.New("cannot find match for action")
	}
	action := string(m[1])
	m = waitIdPat.FindSubmatch(data)
	if m == nil {
		return errgo.New("cannot find match for waitid")
	}
	waitId := string(m[1])

	actionURL, err := url.Parse(action)
	if err != nil {
		return errgo.Notef(err, "cannot parse action URL %q", action)
	}

	// Now simulate the user clicking on "Log in".
	postURL := loginURL.ResolveReference(actionURL)
	log.Printf("posting to %s (waitId %s)", postURL, waitId)
	postResp, err := client.PostForm(postURL.String(), url.Values{
		"user":     {"root"},
		"password": {"superman"},
		"waitid":   {waitId},
	})
	if err != nil {
		return errgo.Notef(err, "cannot post")
	}
	defer postResp.Body.Close()
	if postResp.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(postResp.Body)
		if err != nil {
			return errgo.Notef(err, "cannot read body")
		}
		return errgo.Newf("post failed with status %s (body %q)", postResp.Status, body)
	}
	return nil
}
