package macaroon_test

import (
	"encoding/hex"
	"encoding/json"
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
	m := macaroon.New(rootKey, "some id", "a location")
	c.Assert(m.Location(), gc.Equals, "a location")
	c.Assert(string(m.Id()), gc.Equals, "some id")

	ok, err := m.Verify(rootKey, never, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(ok, gc.Equals, true)
}

func (*macaroonSuite) TestFirstPartyCaveat(c *gc.C) {
	rootKey := []byte("secret")
	m := macaroon.New(rootKey, "some id", "a location")

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
	m := macaroon.New(rootKey, "some id", "a location")

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
	m0 := macaroon.New(rootKey, "some id", "a location")
	m0.AddFirstPartyCaveat("account = 3735928559")
	m0JSON, err := json.Marshal(m0)
	c.Assert(err, gc.IsNil)
	var m1 macaroon.Macaroon
	err = json.Unmarshal(m0JSON, &m1)
	c.Assert(err, gc.IsNil)
	c.Assert(m0.Location(), gc.Equals, m1.Location())
	c.Assert(m0.Id(), gc.Equals, m1.Id())
	c.Assert(
		hex.EncodeToString(m0.Signature()),
		gc.Equals,
		hex.EncodeToString(m1.Signature()))
}

func (*macaroonSuite) TestUnmarshalJSON(c *gc.C) {
	var original, got macaroon.Macaroon
	jsonData := `{"caveats":[{"cid":"account = 3735928559"},{"cid":"time < 2015-01-01T00:00"},{"cid":"email = alice@example.org"}],"location":"http:\\/\\/mybank\\/","identifier":"we used our secret key","signature":"882e6d59496ed5245edb7ab5b8839ecd63e5d504e54839804f164070d8eed952"}`
	mJSON := []byte(jsonData)
	err := json.Unmarshal(mJSON, &original)
	c.Assert(err, gc.IsNil)
	data, err := original.MarshalJSON()
	c.Assert(err, gc.IsNil)
	err = json.Unmarshal(data, &got)
	c.Assert(err, gc.IsNil)
	c.Assert(got, gc.DeepEquals, original)
	c.Assert(original.Signature(), gc.DeepEquals,
		[]byte{0x88, 0x2e, 0x6d, 0x59, 0x49, 0x6e, 0xd5, 0x24, 0x5e, 0xdb, 0x7a, 0xb5, 0xb8, 0x83,
			0x9e, 0xcd, 0x63, 0xe5, 0xd5, 0x04, 0xe5, 0x48, 0x39, 0x80, 0x4f, 0x16, 0x40, 0x70,
			0xd8, 0xee, 0xd9, 0x52})
}
