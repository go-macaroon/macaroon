package macaroon

import (
	"bytes"
	"fmt"
)

// The macaroon binary encoding is made from a sequence
// of "packets", each of which has a field name and some data.
// The encoding is:
//
// - four ascii hex digits holding the entire packet size (including
// the digits themselves).
//
// - the field name, followed by an ascii space.
//
// - the raw data
//
// For efficiency, we store all the packets inside
// a single byte slice inside the macaroon, Macaroon.data. This
// is reasonable to do because we only ever append
// to macaroons.
//
// The packet struct below holds a reference into Macaroon.data.
type packet struct {
	start     int32
	totalLen  uint16
	headerLen uint16
}

func (p packet) len() int {
	return int(p.totalLen)
}

// dataBytes returns the data payload of the packet.
func (m *Macaroon) dataBytes(p packet) []byte {
	return m.data[p.start+int32(p.headerLen) : p.start+int32(p.totalLen)]
}

// packetBytes returns the entire packet.
func (m *Macaroon) packetBytes(p packet) []byte {
	return m.data[p.start : p.start+int32(p.totalLen)]
}

// fieldName returns the field name of the packet.
func (m *Macaroon) fieldName(p packet) []byte {
	return m.data[p.start+4 : p.start+int32(p.headerLen)-1]
}

// parsePacket parses the packet starting at the given
// index into m.data.
func (m *Macaroon) parsePacket(start int) (packet, error) {
	data := m.data[start:]
	if len(data) < 6 {
		return packet{}, fmt.Errorf("packet too short")
	}
	plen, ok := parseSize(data)
	if !ok {
		return packet{}, fmt.Errorf("cannot parse size")
	}
	if plen > len(data) {
		return packet{}, fmt.Errorf("packet size too big")
	}
	data = data[4:plen]
	i := bytes.IndexByte(data, ' ')
	if i <= 0 {
		return packet{}, fmt.Errorf("cannot parse field name")
	}
	return packet{
		start:     int32(start),
		totalLen:  uint16(plen),
		headerLen: uint16(4 + i + 1),
	}, nil
}

const maxPacketLen = 0xffff

// appendPacket appends a packet with the given field name
// and data to m.data, and returns the packet appended.
func (m *Macaroon) appendPacket(field string, data []byte) (packet, error) {
	plen := 4 + len(field) + 1 + len(data)
	if plen > maxPacketLen {
		return packet{}, fmt.Errorf("field %q is too big for macaroon", field)
	}
	s := packet{
		start:     int32(len(m.data)),
		totalLen:  uint16(plen),
		headerLen: uint16(4 + len(field) + 1),
	}
	m.data = appendSize(m.data, plen)
	m.data = append(m.data, field...)
	m.data = append(m.data, ' ')
	m.data = append(m.data, data...)
	return s, nil
}

var hexDigits = []byte("0123456789abcdef")

func appendSize(data []byte, size int) []byte {
	return append(data,
		hexDigits[size>>12],
		hexDigits[(size>>8)&0xf],
		hexDigits[(size>>4)&0xf],
		hexDigits[size&0xf],
	)
}

func parseSize(data []byte) (int, bool) {
	d0, ok0 := asciiHex(data[0])
	d1, ok1 := asciiHex(data[1])
	d2, ok2 := asciiHex(data[2])
	d3, ok3 := asciiHex(data[3])
	return d0<<12 + d1<<8 + d2<<4 + d3, ok0 && ok1 && ok2 && ok3
}

func asciiHex(b byte) (int, bool) {
	switch {
	case b >= '0' && b <= '9':
		return int(b) - '0', true
	case b >= 'a' && b <= 'f':
		return int(b) - 'a' + 0xa, true
	}
	return 0, false
}
