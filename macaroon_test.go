package macaroon_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	_ "net/http"
	"github.com/rogpeppe/macaroon"
	gc "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type macaroonSuite struct{}

var _ = gc.Suite(&macaroonSuite{})

func never(string) (bool, error) {
	return false, nil
}

func (*macaroonSuite) TestNoCaveats(c *gc.C) {
	rootKey := []byte("secret")
	m := macaroon.New(rootKey, []byte("some id"), "a location")
	c.Assert(m.Location(), gc.Equals, "a location")
	c.Assert(string(m.Id()), gc.Equals, "some id")

	ok, err := m.Verify(rootKey, never, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(ok, gc.Equals, true)
}

func (*macaroonSuite) TestFirstPartyCaveat(c *gc.C) {
	rootKey := []byte("secret")
	m := macaroon.New(rootKey, []byte("some id"), "a location")

	caveats := map[string]bool{
		"a caveat":       true,
		"another caveat": true,
	}
	tested := make(map[string]bool)

	for cav := range caveats {
		m.AddFirstPartyCaveat(cav)
	}

	check := func(cav string) (bool, error) {
		tested[cav] = true
		return caveats[cav], nil
	}
	ok, err := m.Verify(rootKey, check, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(ok, gc.Equals, true)

	c.Assert(tested, gc.DeepEquals, caveats)
}

func (*macaroonSuite) TestThirdPartyCaveat(c *gc.C) {
	rootKey := []byte("secret")
	m := macaroon.New(rootKey, []byte("some id"), "a location")

	sharedSecret := []byte("shared secret")
	id, err := m.AddThirdPartyCaveat(sharedSecret, "3rd party caveat", "remote.com")
	c.Assert(err, gc.IsNil)

	// This section would be done on the third party server.
	caveat, err := macaroon.DecryptThirdPartyCaveatId(sharedSecret, id)
	c.Assert(err, gc.IsNil)

	dm := macaroon.New(caveat.RootKey, id, "remote location")
	dm.Bind(m.Signature())
	ok, err := m.Verify(rootKey, never, map[string]*macaroon.Macaroon{
		string(id): dm,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(ok, gc.Equals, true)
}

func (*macaroonSuite) TestMarshalJSON(c *gc.C) {
	rootKey := []byte("secret")
	m0 := macaroon.New(rootKey, []byte("some id"), "a location")
	m0.AddFirstPartyCaveat("account = 3735928559")
	m0Json, err := json.Marshal(m0)
	c.Assert(err, gc.IsNil)
	m1 := macaroon.Macaroon{}
	err = json.Unmarshal(m0Json, &m1)
	c.Assert(err, gc.IsNil)
	c.Assert(m0.Location(), gc.Equals, m1.Location())
	c.Assert(
		base64.StdEncoding.EncodeToString(m0.Id()),
		gc.Equals,
		base64.StdEncoding.EncodeToString(m1.Id()))
	c.Assert(
		base64.StdEncoding.EncodeToString(m0.Signature()),
		gc.Equals,
		base64.StdEncoding.EncodeToString(m1.Signature()))

	fmt.Printf("%#v\n", *m0)
	fmt.Printf("%s\n", string(m0Json))
	fmt.Printf("%#v\n", m1)

}
