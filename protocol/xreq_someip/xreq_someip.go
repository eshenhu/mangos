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

// Packege xreq_someip implements the SOME/IP protocol. This sends messages
// out to xrep_someip partners, and receives their responses and notifications.
package xreq_someip

import (
	"encoding/binary"
	"sync"
	"time"

	"go.nanomsg.org/mangos/v3/protocol"
	"go.nanomsg.org/mangos/v3/protocol/proto_someip"
)

// Protocol identity information.
const (
	Self       = protocol.ProtoSomeIPReq
	Peer       = protocol.ProtoSomeIPRep
	SelfName   = "someip-req"
	PeerName   = "someip-rep"
	NotifyName = "someip-notify"
)

type SomeIPOpts = proto_someip.SomeIPOpts
type MessageSomeIP = proto_someip.MessageSomeIP

const defaultSurveyTime = time.Second
const defaultQLen = 128

type pipe struct {
	s      *socket
	p      protocol.Pipe
	closeQ chan struct{}
}

type context struct {
	s *socket

	recvQ      chan *MessageSomeIP
	recvExpire time.Duration
	recvQLen   int

	sessionID uint16
	chkMsg    func(c *context, m *MessageSomeIP) error
}

type socket struct {
	opts SomeIPOpts
	//
	master *context // default context
	slave  *context // background context for receiving notification
	//
	closed bool // true if closed
	closeQ chan struct{}
	//
	sendQ      chan *protocol.Message // sendQ
	sendExpire time.Duration
	sendQLen   int // send Q depth
	//
	sizeQ chan struct{}
	//
	sync.Mutex
}

var (
	nilQ <-chan time.Time
)

func (c *context) SendMsg(m *protocol.Message) error {
	s := c.s
	s.Lock()

	if s.closed {
		m.Free()
		s.Unlock()
		return protocol.ErrClosed
	}

	if len(m.Header) != proto_someip.RzvHdrBySomeIP || len(m.Body) < proto_someip.RzvBodyBySomeIP {
		m.Free()
		s.Unlock()
		return protocol.ErrTooShort
	}

	timeQ := nilQ
	if s.sendExpire > 0 {
		timeQ = time.After(s.sendExpire)
	}
	sendQ := s.sendQ
	closeQ := s.closeQ
	sizeQ := s.sizeQ
	s.Unlock()

	//increment by one on every call
	c.sessionID = c.sessionID + 1

	//install the L2 information
	binary.BigEndian.PutUint16(m.Body[0:2], s.opts.ServiceID)
	//m.Body[2] = m.Header[0]
	//m.Body[3] = m.Header[1]
	binary.BigEndian.PutUint32(m.Body[4:8], (uint32)(len(m.Body)-8))
	binary.BigEndian.PutUint16(m.Body[8:10], s.opts.ClientID)
	binary.BigEndian.PutUint16(m.Body[10:12], c.sessionID)
	m.Body[12] = s.opts.ProtoVersion
	m.Body[13] = s.opts.InfVersion
	//m.Body[14] = m.Header[2]
	m.Body[15] = 0

	select {
	case sendQ <- m:
		return nil
	case <-closeQ:
		return protocol.ErrClosed
	case <-timeQ:
		return protocol.ErrSendTimeout
	case <-sizeQ:
		m.Free()
		return nil
	}
}

func (c *context) RecvMsg() (*protocol.Message, error) {
	for {
		s := c.s
		s.Lock()
		if s.closed {
			s.Unlock()
			return nil, protocol.ErrClosed
		}
		timeq := nilQ
		if c.recvExpire > 0 {
			timeq = time.After(c.recvExpire)
		}
		sizeQ := s.sizeQ
		s.Unlock()

		select {
		case <-s.closeQ:
			return nil, protocol.ErrClosed
		case m := <-c.recvQ:
			if m == nil {
				return nil, protocol.ErrBadValue
			}
			if err := c.chkMsg(c, m); err != nil {
				m.M.Free()
				return nil, err
			}
			return m.M, nil
		case <-timeq:
			return nil, protocol.ErrRecvTimeout
		case <-sizeQ:
			continue
		}
	}
}

func (c *context) close() {
}

func (c *context) Close() error {
	c.s.Lock()
	defer c.s.Unlock()
	c.close()
	return nil
}

func (c *context) SetOption(option string, value interface{}) error {
	switch option {
	case protocol.OptionReadQLen:
		if v, ok := value.(int); ok && v >= 0 {
			c.s.Lock()
			c.recvQLen = v
			c.s.Unlock()
			return nil
		}
		return protocol.ErrBadValue
	}
	return protocol.ErrBadOption
}

func (c *context) GetOption(option string) (interface{}, error) {
	switch option {
	case protocol.OptionReadQLen:
		c.s.Lock()
		v := c.recvQLen
		c.s.Unlock()
		return v, nil

	case protocol.OptionLinkSz:
		return (int)(16), nil
	}
	return nil, protocol.ErrBadOption
}

func (p *pipe) close() {
	_ = p.p.Close()
}

func (p *pipe) sender() {
	s := p.s
outer:
	for {
		var m *protocol.Message
		select {
		case <-p.closeQ:
			break outer
		case m = <-s.sendQ:
		}

		if err := p.p.SendMsg(m); err != nil {
			m.Free()
			break
		}
	}
	p.close()
}

func (p *pipe) receiver() {
	s := p.s
outer:
	for {
		m := p.p.RecvMsg()
		if m == nil {
			break
		}
		m2 := &MessageSomeIP{
			M:            m,
			ServiceID:    binary.BigEndian.Uint16(m.Body[0:2]),
			MethodID:     binary.BigEndian.Uint16(m.Body[2:4]),
			ClientID:     binary.BigEndian.Uint16(m.Body[8:10]),
			SessionID:    binary.BigEndian.Uint16(m.Body[10:12]),
			ProtoVersion: m.Body[12],
			InfVersion:   m.Body[13],
			MsgType:      (proto_someip.MsgTypeCode)(m.Body[14]),
			RtnCode:      (proto_someip.ErrCodeSomeIP)(m.Body[15]),
			Payload:      nil,
		}
		if len(m.Body) >= proto_someip.RzvBodyBySomeIP {
			m2.Payload = m.Body[16:]
		}

		var recvQ chan *MessageSomeIP

		s.Lock()
		recvMasterQ := s.master.recvQ
		recvSlaveQ := s.slave.recvQ
		sizeQ := s.sizeQ
		s.Unlock()

		switch m2.MsgType {
		case proto_someip.MT_NOTIFICATION:
			recvQ = recvSlaveQ
		case proto_someip.MT_RESPONSE:
			fallthrough
		case proto_someip.MT_ERROR:
			recvQ = recvMasterQ
		default:
			continue
		}

		select {
		case recvQ <- m2:
			continue
		case <-sizeQ: // resize discards
			m.Free()
			continue
		case <-p.closeQ:
			m.Free()
			break outer
		}
	}
	p.close()
}

func (s *socket) OpenContext() (protocol.Context, error) {
	return nil, protocol.ErrProtoOp
}

// SendMsg: m.Body[0:2]  ~ MethodID  m.Body[2:] Payload
// Should NOT USE directly. make test only
func (s *socket) SendMsg(m *protocol.Message) error {
	return s.master.SendMsg(m)
}

func (s *socket) RecvMsg() (*protocol.Message, error) {
	return s.master.RecvMsg()
}

func (s *socket) AddPipe(pp protocol.Pipe) error {
	p := &pipe{
		p:      pp,
		s:      s,
		closeQ: make(chan struct{}),
	}
	pp.SetPrivate(p)
	s.Lock()
	defer s.Unlock()
	if s.closed {
		return protocol.ErrClosed
	}
	go p.receiver()
	go p.sender()
	return nil
}

func (s *socket) RemovePipe(pp protocol.Pipe) {
	p := pp.GetPrivate().(*pipe)
	close(p.closeQ)
	s.Lock()
	s.Unlock()
}

func (s *socket) Close() error {
	s.Lock()
	if s.closed {
		s.Unlock()
		return protocol.ErrClosed
	}
	s.closed = true
	s.master.close()
	s.slave.close()
	s.Unlock()
	close(s.closeQ)
	return nil
}

func (s *socket) GetOption(option string) (interface{}, error) {
	switch option {
	case protocol.OptionRaw:
		return false, nil
	case protocol.OptionWriteQLen:
		s.Lock()
		v := s.sendQLen
		s.Unlock()
		return v, nil

	case protocol.OptionRecvDeadline:
		s.Lock()
		v := s.master.recvExpire
		s.Unlock()
		return v, nil

	case protocol.OptionSendDeadline:
		s.Lock()
		v := s.sendExpire
		s.Unlock()
		return v, nil

	case protocol.OptionSomeIPCtxMaster:
		s.Lock()
		v := s.master
		s.Unlock()
		return v, nil

	case protocol.OptionSomeIPCtxSlave:
		s.Lock()
		v := s.slave
		s.Unlock()
		return v, nil

	case protocol.OptionSomeIPSrvID:
		s.Lock()
		v := s.opts.ServiceID
		s.Unlock()
		return v, nil

	default:
		return s.master.GetOption(option)
	}
}

func (s *socket) SetOption(option string, value interface{}) error {
	switch option {
	case protocol.OptionWriteQLen:
		if v, ok := value.(int); ok && v >= 0 {
			newQ := make(chan *protocol.Message, v)
			sizeQ := make(chan struct{})
			s.Lock()
			s.sendQLen = v
			s.sendQ = newQ
			sizeQ, s.sizeQ = s.sizeQ, sizeQ
			s.Unlock()
			close(sizeQ)
			return nil
		}
		return protocol.ErrBadValue

	case protocol.OptionRecvDeadline:
		if v, ok := value.(time.Duration); ok {
			s.Lock()
			s.master.recvExpire = v
			s.Unlock()
			return nil
		}
		return protocol.ErrBadValue

	case protocol.OptionSendDeadline:
		if v, ok := value.(time.Duration); ok {
			s.Lock()
			s.sendExpire = v
			s.Unlock()
			return nil
		}
		return protocol.ErrBadValue

	case protocol.OptionReadQLen:
		if v, ok := value.(int); ok && v >= 0 {
			recvQ := make(chan *MessageSomeIP, v)
			sizeQ := make(chan struct{})
			s.Lock()
			s.master.recvQLen = v
			s.master.recvQ = recvQ
			sizeQ, s.sizeQ = s.sizeQ, sizeQ
			s.Unlock()
			close(sizeQ)
			return nil
		}
		return protocol.ErrBadValue

	case protocol.OptionSomeIPPV:
		if v, ok := value.(uint8); ok {
			s.Lock()
			s.opts.ProtoVersion = v
			s.Unlock()
			return nil
		}
		return protocol.ErrBadValue

	case protocol.OptionSomeIPInfPV:
		if v, ok := value.(uint8); ok {
			s.Lock()
			s.opts.InfVersion = v
			s.Unlock()
			return nil
		}
		return protocol.ErrBadValue

	case protocol.OptionSomeIPSrvID:
		if v, ok := value.(uint16); ok {
			s.Lock()
			s.opts.ServiceID = v
			s.Unlock()
			return nil
		}
		return protocol.ErrBadValue

	case protocol.OptionSomeIPCltID:
		if v, ok := value.(uint16); ok {
			s.Lock()
			s.opts.ClientID = v
			s.Unlock()
			return nil
		}
		return protocol.ErrBadValue

	default:
		return protocol.ErrBadOption
	}
}

func (*socket) Info() protocol.Info {
	return protocol.Info{
		Self:     Self,
		Peer:     Peer,
		SelfName: SelfName,
		PeerName: PeerName,
	}
}

func chkMsgCommon(c *context, m *MessageSomeIP) error {
	s := c.s
	if s.opts.ServiceID != m.ServiceID || s.opts.ClientID != m.ClientID ||
		s.opts.ProtoVersion != m.ProtoVersion || s.opts.InfVersion != m.InfVersion {
		return protocol.ErrProtoOp
	}
	return nil
}

func chkMsgExchange(c *context, m *MessageSomeIP) error {
	if err := chkMsgCommon(c, m); err != nil {
		return err
	}
	if m.MsgType == proto_someip.MT_RESPONSE && c.sessionID != m.SessionID {
		return protocol.ErrProtoOp
	}
	return nil
}

func chkMsgNotification(c *context, m *MessageSomeIP) error {
	return chkMsgCommon(c, m)
}

// NewProtocol returns a new protocol implementation.
func NewProtocol() protocol.Protocol {
	s := &socket{
		sendQLen: defaultQLen,
		sendQ:    make(chan *protocol.Message, defaultQLen),
		closeQ:   make(chan struct{}),
		sizeQ:    make(chan struct{}),
	}
	s.master = &context{
		s:          s,
		recvExpire: 500 * time.Millisecond,
		recvQLen:   defaultQLen,
		recvQ:      make(chan *MessageSomeIP, defaultQLen),
		//sessionID:  (uint16)(time.Now().UnixNano()),
		sessionID: 0,
		chkMsg:    chkMsgExchange,
	}

	s.slave = &context{
		s:          s,
		recvExpire: 0,
		recvQLen:   defaultQLen,
		recvQ:      make(chan *MessageSomeIP, defaultQLen),
		sessionID:  0,
		chkMsg:     chkMsgNotification,
	}
	return s
}

// NewSocket allocates a new Socket using the RESPONDENT protocol.
func NewSocket() (protocol.Socket, error) {
	return protocol.MakeSocket(NewProtocol()), nil
}
