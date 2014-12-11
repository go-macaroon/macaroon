package macaroon_test

import (
	gc "gopkg.in/check.v1"

	"gopkg.in/macaroon.v1"
)

type marshalSuite struct{}

var _ = gc.Suite(&marshalSuite{})

func (*marshalSuite) TestMarshalUnmarshalMacaroon(c *gc.C) {
	rootKey := []byte("secret")
	m := MustNew(rootKey, "some id", "a location")

	err := m.AddFirstPartyCaveat("a caveat")
	c.Assert(err, gc.IsNil)

	b, err := m.MarshalBinary()
	c.Assert(err, gc.IsNil)

	unmarshaledM := &macaroon.Macaroon{}
	err = unmarshaledM.UnmarshalBinary(b)
	c.Assert(err, gc.IsNil)

	c.Assert(m.Location(), gc.Equals, unmarshaledM.Location())
	c.Assert(m.Id(), gc.Equals, unmarshaledM.Id())
	c.Assert(m.Signature(), gc.DeepEquals, unmarshaledM.Signature())
	c.Assert(m.Caveats(), gc.DeepEquals, unmarshaledM.Caveats())
	c.Assert(m, gc.DeepEquals, unmarshaledM)
}

func (*marshalSuite) TestMarshalUnmarshalSlice(c *gc.C) {
	rootKey := []byte("secret")
	m1 := MustNew(rootKey, "some id", "a location")
	m2 := MustNew(rootKey, "some other id", "another location")

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
		c.Assert(m.Location(), gc.Equals, unmarshaledMacs[i].Location())
		c.Assert(m.Id(), gc.Equals, unmarshaledMacs[i].Id())
		c.Assert(m.Signature(), gc.DeepEquals, unmarshaledMacs[i].Signature())
		c.Assert(m.Caveats(), gc.DeepEquals, unmarshaledMacs[i].Caveats())
	}
	c.Assert(macaroons, gc.DeepEquals, unmarshaledMacs)

	// The unmarshaled macaroons share the same underlying data
	// slice, so check that appending a caveat to the first does not
	// affect the second.
	for i := 0; i < 10; i++ {
		err = unmarshaledMacs[0].AddFirstPartyCaveat("caveat")
		c.Assert(err, gc.IsNil)
	}
	c.Assert(unmarshaledMacs[1], gc.DeepEquals, macaroons[1])
	c.Assert(err, gc.IsNil)
}

func (*marshalSuite) TestSliceRoundtrip(c *gc.C) {
	rootKey := []byte("secret")
	m1 := MustNew(rootKey, "some id", "a location")
	m2 := MustNew(rootKey, "some other id", "another location")

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

	c.Assert(b, gc.DeepEquals, marshaledMacs)
}
