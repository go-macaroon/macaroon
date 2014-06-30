package macaroon

import (
	gc "gopkg.in/check.v1"

	"strings"
)

type packetSuite struct{}

var _ = gc.Suite(&packetSuite{})

func (*packetSuite) TestAppendPacket(c *gc.C) {
	var m Macaroon
	p, ok := m.appendPacket("field", []byte("some data"))
	c.Assert(ok, gc.Equals, true)
	c.Assert(string(m.data), gc.Equals, "0013field some data")
	c.Assert(p, gc.Equals, packet{
		start:     0,
		totalLen:  19,
		headerLen: 10,
	})

	p, ok = m.appendPacket("otherfield", []byte("more and more data"))
	c.Assert(ok, gc.Equals, true)
	c.Assert(string(m.data), gc.Equals, "0013field some data0021otherfield more and more data")
	c.Assert(p, gc.Equals, packet{
		start:     19,
		totalLen:  33,
		headerLen: 15,
	})
}

func (*packetSuite) TestAppendPacketTooBig(c *gc.C) {
	var m Macaroon
	data := make([]byte, 65532)
	p, ok := m.appendPacket("field", data)
	c.Assert(ok, gc.Equals, false)
	c.Assert(p, gc.Equals, packet{})
}

func (*packetSuite) TestDataBytes(c *gc.C) {
	var m Macaroon
	m.appendPacket("first", []byte("first data"))
	p, ok := m.appendPacket("field", []byte("some data"))
	c.Assert(ok, gc.Equals, true)
	c.Assert(string(m.dataBytes(p)), gc.Equals, "some data")
}

func (*packetSuite) TestPacketBytes(c *gc.C) {
	var m Macaroon
	m.appendPacket("first", []byte("first data"))
	p, ok := m.appendPacket("field", []byte("some data"))
	c.Assert(ok, gc.Equals, true)
	c.Assert(string(m.packetBytes(p)), gc.Equals, "0013field some data")
}

func (*packetSuite) TestFieldName(c *gc.C) {
	var m Macaroon
	m.appendPacket("first", []byte("first data"))
	p, ok := m.appendPacket("field", []byte("some data"))
	c.Assert(ok, gc.Equals, true)
	c.Assert(string(m.fieldName(p)), gc.Equals, "field")
}

var parsePacketTests = []struct {
	data        string
	start       int
	expect      packet
	expectErr   string
	expectData  string
	expectField string
}{{
	expectErr: "packet too short",
}, {
	data:  "0013field some data",
	start: 0,
	expect: packet{
		start:     0,
		totalLen:  19,
		headerLen: 10,
	},
	expectData:  "some data",
	expectField: "field",
}, {
	data:      "0013field some data",
	start:     1,
	expectErr: "packet size too big",
}, {
	data:  "0013field some data0013field some data",
	start: 0x13,
	expect: packet{
		start:     0x13,
		totalLen:  19,
		headerLen: 10,
	},
	expectData:  "some data",
	expectField: "field",
}, {
	data:      "0013fieldwithoutanyspaceordata",
	start:     0,
	expectErr: "cannot parse field name",
}, {
	data:  "fedcsomefield " + strings.Repeat("x", 0xfedc-4-len("somefield ")),
	start: 0,
	expect: packet{
		start:     0,
		totalLen:  0xfedc,
		headerLen: 14,
	},
	expectData:  strings.Repeat("x", 0xfedc-4-len("somefield ")),
	expectField: "somefield",
}}

func (*packetSuite) TestParsePacket(c *gc.C) {
	for i, test := range parsePacketTests {
		c.Logf("test %d: %q", i, truncate(test.data))
		m := Macaroon{
			data: []byte(test.data),
		}
		p, err := m.parsePacket(test.start)
		if test.expectErr != "" {
			c.Assert(err, gc.ErrorMatches, test.expectErr)
			c.Assert(p, gc.Equals, packet{})
			continue
		}
		c.Assert(err, gc.IsNil)
		c.Assert(p, gc.Equals, test.expect)
		c.Assert(string(m.dataBytes(p)), gc.Equals, test.expectData)
		c.Assert(string(m.fieldName(p)), gc.Equals, test.expectField)

		// append the same packet again and check that
		// the contents are the same.
		p1, ok := m.appendPacket(test.expectField, []byte(test.expectData))
		c.Assert(ok, gc.Equals, true)
		c.Assert(string(m.packetBytes(p)), gc.Equals, string(m.packetBytes(p1)))
	}
}

func truncate(d string) string {
	if len(d) > 50 {
		return d[0:50] + "..."
	}
	return d
}
