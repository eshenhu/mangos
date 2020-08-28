// Copyright 2019 The Mangos Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use file except in compliance with the License.
// You may obtain a copy of the license at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package transport

import (
	"encoding/binary"
	"io"
	"net"
	"sync"

	"go.nanomsg.org/mangos/v3"
)

// connSomeip implements the Pipe interface on top of net.Conn.  The
// assumption is that transports using this have similar wire protocols,
// and connSomeip is meant to be used as a building block.
type connSomeip struct {
	c       net.Conn
	proto   ProtocolInfo
	open    bool
	options map[string]interface{}
	maxrx   int
	sync.Mutex
}

// Recv implements the TranPipe Recv method.  The message received is expected
// as a 64-bit size (network byte order) followed by the message itself.
func (p *connSomeip) Recv() (*Message, error) {
	var err error
	var msg *Message
	var h [8]byte

	if _, err = io.ReadFull(p.c, h[0:8]); err != nil {
		return nil, err
	}

	sz := binary.BigEndian.Uint32(h[4:8])

	// refer to Figure-4.1. sz must be greater or equal than 8
	if sz < 8 || (p.maxrx > 0 && sz > (uint32)(p.maxrx)) {
		return nil, mangos.ErrTooLong
	}

	szPlusHdr := sz + 8
	if szPlusHdr >= 1<<31 {
		return nil, mangos.ErrTooLong
	}

	msg = mangos.NewMessage((int)(szPlusHdr))
	// 0~3: store pipe id
	msg.Header = msg.Header[0:4]
	msg.Body = msg.Body[0:szPlusHdr]
	copy(msg.Body[0:4], h[0:3])

	if _, err = io.ReadFull(p.c, msg.Body[8:]); err != nil {
		msg.Free()
		return nil, err
	}
	return msg, nil
}

// Send implements the Pipe Send method.  The message is sent as a 64-bit
// size (network byte order) followed by the message itself.
func (p *connSomeip) Send(msg *Message) error {
	if _, err := p.c.Write(msg.Body[:]); err != nil {
		return err
	}
	msg.Free()
	return nil
}

// Close implements the Pipe Close method.
func (p *connSomeip) Close() error {
	p.Lock()
	defer p.Unlock()
	if p.open {
		p.open = false
		return p.c.Close()
	}
	return nil
}

func (p *connSomeip) GetOption(n string) (interface{}, error) {
	switch n {
	case mangos.OptionMaxRecvSize:
		return p.maxrx, nil
	}
	if v, ok := p.options[n]; ok {
		return v, nil
	}
	return nil, mangos.ErrBadProperty
}

// ConnPipeSomeIP is used for stream oriented transports.  It is a superset of
// Pipe, but adds methods specific for transports.
type ConnPipeSomeIP interface {

	// SetMaxRecvSize is used to set the maximum receive size.
	SetMaxRecvSize(int)

	Pipe
}

// NewConnPipeSomeIP allocates a new Pipe using the supplied net.Conn, and
// initializes it.  It performs no negotiation -- use a Handshaker to
// arrange for that.
//
// Stream oriented transports can utilize this to implement a Transport.
// The implementation will also need to implement PipeDialer, PipeAccepter,
// and the Transport enclosing structure.   Using this layered interface,
// the implementation needn't bother concerning itself with passing actual
// SP messages once the lower layer connection is established.
func NewConnPipeSomeIP(c net.Conn, proto ProtocolInfo, options map[string]interface{}) ConnPipeSomeIP {
	p := &connSomeip{
		c:       c,
		proto:   proto,
		options: make(map[string]interface{}),
	}

	p.options[mangos.OptionMaxRecvSize] = 0
	p.options[mangos.OptionLocalAddr] = c.LocalAddr()
	p.options[mangos.OptionRemoteAddr] = c.RemoteAddr()
	for n, v := range options {
		p.options[n] = v
	}
	p.maxrx = p.options[mangos.OptionMaxRecvSize].(int)

	return p
}

func (p *connSomeip) SetMaxRecvSize(sz int) {
	p.maxrx = sz
}
