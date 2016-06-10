package macaroon_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"gopkg.in/macaroon.v2-unstable"
)

type marshalSuite struct{}

var _ = gc.Suite(&marshalSuite{})

func (s *marshalSuite) TestMarshalUnmarshalMacaroonV1(c *gc.C) {
	s.testMarshalUnmarshalWithVersion(c, macaroon.MarshalV1)
}

func (s *marshalSuite) TestMarshalUnmarshalMacaroonV2(c *gc.C) {
	s.testMarshalUnmarshalWithVersion(c, macaroon.MarshalV2)
}

func (*marshalSuite) testMarshalUnmarshalWithVersion(c *gc.C, vers macaroon.MarshalOpts) {
	rootKey := []byte("secret")
	m := MustNew(rootKey, []byte("some id"), "a location")
	m.MarshalAs(vers)

	// Adding the third party caveat before the first party caveat
	// tests a former bug where the caveat wasn't zeroed
	// before moving to the next caveat.
	err := m.AddThirdPartyCaveat([]byte("shared root key"), []byte("3rd party caveat"), "remote.com")
	c.Assert(err, gc.IsNil)

	err = m.AddFirstPartyCaveat("a caveat")
	c.Assert(err, gc.IsNil)

	b, err := m.MarshalBinary()
	c.Assert(err, gc.IsNil)

	var um macaroon.Macaroon
	err = um.UnmarshalBinary(b)
	c.Assert(err, gc.IsNil)

	c.Assert(um.Location(), gc.Equals, m.Location())
	c.Assert(string(um.Id()), gc.Equals, string(m.Id()))
	c.Assert(um.Signature(), jc.DeepEquals, m.Signature())
	c.Assert(um.Caveats(), jc.DeepEquals, m.Caveats())
	c.Assert(um.UnmarshaledAs(), gc.Equals, vers)
	um.SetUnmarshaledAs(m.UnmarshaledAs())
	um.MarshalAs(vers)
	c.Assert(m, jc.DeepEquals, &um)
}

func (s *marshalSuite) TestMarshalUnmarshalSliceV1(c *gc.C) {
	s.testMarshalUnmarshalSliceWithVersion(c, macaroon.MarshalV1)
}

func (s *marshalSuite) TestMarshalUnmarshalSliceV2(c *gc.C) {
	s.testMarshalUnmarshalSliceWithVersion(c, macaroon.MarshalV2)
}

func (*marshalSuite) testMarshalUnmarshalSliceWithVersion(c *gc.C, vers macaroon.MarshalOpts) {
	rootKey := []byte("secret")
	m1 := MustNew(rootKey, []byte("some id"), "a location")
	m1.MarshalAs(vers)
	m2 := MustNew(rootKey, []byte("some other id"), "another location")
	m2.MarshalAs(vers)

	err := m1.AddFirstPartyCaveat("a caveat")
	c.Assert(err, gc.IsNil)
	err = m2.AddFirstPartyCaveat("another caveat")
	c.Assert(err, gc.IsNil)

	macaroons := macaroon.Slice{m1, m2}

	b, err := macaroons.MarshalBinary()
	c.Assert(err, gc.IsNil)

	var unmarshaledMacs macaroon.Slice
	err = unmarshaledMacs.UnmarshalBinary(b)
	c.Assert(err, gc.IsNil)

	c.Assert(unmarshaledMacs, gc.HasLen, len(macaroons))
	for i, m := range macaroons {
		um := unmarshaledMacs[i]
		c.Assert(um.Location(), gc.Equals, m.Location())
		c.Assert(string(um.Id()), gc.Equals, string(m.Id()))
		c.Assert(um.Signature(), jc.DeepEquals, m.Signature())
		c.Assert(um.Caveats(), jc.DeepEquals, m.Caveats())
		c.Assert(um.UnmarshaledAs(), gc.Equals, vers)
		um.MarshalAs(vers)
		um.SetUnmarshaledAs(m.UnmarshaledAs())
	}
	c.Assert(macaroons, jc.DeepEquals, unmarshaledMacs)

	// Check that appending a caveat to the first does not
	// affect the second.
	for i := 0; i < 10; i++ {
		err = unmarshaledMacs[0].AddFirstPartyCaveat("caveat")
		c.Assert(err, gc.IsNil)
	}
	unmarshaledMacs[1].SetUnmarshaledAs(macaroons[1].UnmarshaledAs())
	c.Assert(unmarshaledMacs[1], jc.DeepEquals, macaroons[1])
	c.Assert(err, gc.IsNil)
}

func (s *marshalSuite) TestSliceRoundTripV1(c *gc.C) {
	s.testSliceRoundTripWithVersion(c, macaroon.MarshalV1)
}

func (s *marshalSuite) TestSliceRoundTripV2(c *gc.C) {
	s.testSliceRoundTripWithVersion(c, macaroon.MarshalV2)
}

func (*marshalSuite) testSliceRoundTripWithVersion(c *gc.C, vers macaroon.MarshalOpts) {
	rootKey := []byte("secret")
	m1 := MustNew(rootKey, []byte("some id"), "a location")
	m2 := MustNew(rootKey, []byte("some other id"), "another location")

	err := m1.AddFirstPartyCaveat("a caveat")
	c.Assert(err, gc.IsNil)
	err = m2.AddFirstPartyCaveat("another caveat")
	c.Assert(err, gc.IsNil)

	macaroons := macaroon.Slice{m1, m2}

	b, err := macaroons.MarshalBinary()
	c.Assert(err, gc.IsNil)

	var unmarshaledMacs macaroon.Slice
	err = unmarshaledMacs.UnmarshalBinary(b)
	c.Assert(err, gc.IsNil)

	marshaledMacs, err := unmarshaledMacs.MarshalBinary()
	c.Assert(err, gc.IsNil)

	c.Assert(b, jc.DeepEquals, marshaledMacs)
}
