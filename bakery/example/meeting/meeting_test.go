package meeting_test

import (
	"time"

	gc "gopkg.in/check.v1"

	"github.com/rogpeppe/macaroon/bakery/example/meeting"
)

type suite struct{}

var _ = gc.Suite(&suite{})

func (*suite) TestRendezvousWaitBeforeDone(c *gc.C) {
	m := meeting.New()
	id, err := m.NewRendezvous([]byte("first data"))
	c.Assert(err, gc.IsNil)
	c.Assert(id, gc.Not(gc.Equals), "")

	waitDone := make(chan struct{})
	go func() {
		data0, data1, err := m.Wait(id)
		c.Check(err, gc.IsNil)
		c.Check(string(data0), gc.Equals, "first data")
		c.Check(string(data1), gc.Equals, "second data")

		close(waitDone)
	}()

	time.Sleep(10 * time.Millisecond)
	err = m.Done(id, []byte("second data"))
	c.Assert(err, gc.IsNil)
	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		c.Errorf("timed out waiting for rendezvous")
	}

	// Check that item has now been deleted.
	data0, data1, err := m.Wait(id)
	c.Assert(data0, gc.IsNil)
	c.Assert(data1, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `rendezvous ".*" not found`)
}

func (*suite) TestRendezvousDoneBeforeWait(c *gc.C) {
	m := meeting.New()
	id, err := m.NewRendezvous([]byte("first data"))
	c.Assert(err, gc.IsNil)
	c.Assert(id, gc.Not(gc.Equals), "")

	err = m.Done(id, []byte("second data"))
	c.Assert(err, gc.IsNil)

	err = m.Done(id, []byte("other second data"))
	c.Assert(err, gc.ErrorMatches, `rendezvous ".*" done twice`)

	data0, data1, err := m.Wait(id)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data0), gc.Equals, "first data")
	c.Assert(string(data1), gc.Equals, "second data")

	// Check that item has now been deleted.
	data0, data1, err = m.Wait(id)
	c.Assert(data0, gc.IsNil)
	c.Assert(data1, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `rendezvous ".*" not found`)
}
