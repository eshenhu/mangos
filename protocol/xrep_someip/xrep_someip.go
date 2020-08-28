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

// Package xrep_someip implements the raw REP protocol, which is the response side of
// the request/response pattern.  (REQ is the request.)

package xrep_someip

import (
	"encoding/binary"
	"sync"
	"time"

	"go.nanomsg.org/mangos/v3/protocol"
	"go.nanomsg.org/mangos/v3/protocol/proto_someip"
)

// Protocol identity information.
const (
	Self       = protocol.ProtoSomeIPRep
	Peer       = protocol.ProtoSomeIPReq
	SelfName   = "someip-rep"
	PeerName   = "someip-req"
	NotifyName = "someip-notify"
)

type someIPOpts = proto_someip.SomeIPOpts
type messageSomeIP = proto_someip.MessageSomeIP

type pipe struct {
	p      protocol.Pipe
	s      *socket
	closeQ chan struct{}
	sendQ  chan *protocol.Message
}

type socket struct {
	closed     bool
	closeQ     chan struct{}
	sizeQ      chan struct{}
	recvQ      chan *messageSomeIP
	pipes      map[uint32]*pipe
	recvExpire time.Duration
	sendExpire time.Duration
	sendQLen   int
	recvQLen   int
	bestEffort bool
	ttl        int
	opts       someIPOpts
	chkMsg     func(*socket, *messageSomeIP) (*protocol.Message, error)
	sync.Mutex
}

var (
	nilQ    <-chan time.Time
	closedQ chan time.Time
)

const defaultQLen = 128

func init() {
	closedQ = make(chan time.Time)
	close(closedQ)
}

// SendMessage implements sending a message.  The message must already
// have headers... the first 4 bytes are the identity of the pipe
// we should send to.
// Requirement:
// 1. Store the pipeid in the Header[0:4]
// 2. Store the data which you send in the Payload[16:]
func (s *socket) SendMsg(m *protocol.Message) error {

	s.Lock()

	if s.closed {
		s.Unlock()
		return protocol.ErrClosed
	}

	// 0~4 pipe id 4 message type 5 return code
	if len(m.Header) != proto_someip.RzvHdrBySomeIP {
		s.Unlock()
		m.Free()
		return protocol.ErrTooShort
	}

	id := binary.BigEndian.Uint32(m.Header)
	hdr := m.Header
	//m.Header = m.Header[4:]

	binary.BigEndian.PutUint32(m.Body[4:8], (uint32)(len(m.Body)-8))
	//m.Body[14] = m.Header[0]
	//m.Body[15] = m.Header[1]

	bestEffort := s.bestEffort
	tq := nilQ
	p, ok := s.pipes[id]
	if !ok {
		s.Unlock()
		m.Free()
		return nil
	}
	if bestEffort {
		tq = closedQ
	} else if s.sendExpire > 0 {
		tq = time.After(s.sendExpire)
	}
	s.Unlock()

	select {
	case p.sendQ <- m:
		return nil
	case <-p.closeQ:
		// restore the header
		m.Header = hdr
		return protocol.ErrClosed
	case <-tq:
		if bestEffort {
			m.Free()
			return nil
		}
		// restore the header
		m.Header = hdr
		return protocol.ErrSendTimeout
	}
}

func (s *socket) RecvMsg() (*protocol.Message, error) {
	for {
		s.Lock()
		timeQ := nilQ
		recvQ := s.recvQ
		sizeQ := s.sizeQ
		closeQ := s.closeQ
		if s.recvExpire > 0 {
			timeQ = time.After(s.recvExpire)
		}
		s.Unlock()
		select {
		case <-closeQ:
			return nil, protocol.ErrClosed
		case <-timeQ:
			return nil, protocol.ErrRecvTimeout
		case m := <-recvQ:
			if m == nil {
				return nil, protocol.ErrBadValue
			}
			if snd, err := s.chkMsg(s, m); err != nil {
				s.SendMsg(snd)
				return nil, err
			}
			return m.M, nil
		case <-sizeQ:
		}
	}
}

func (p *pipe) receiver() {
	s := p.s
outer:
	for {
		m := p.p.RecvMsg()
		if m == nil {
			break
		}
		// Outer most value of header is pipe ID
		binary.BigEndian.PutUint32(m.Header, p.p.ID())
		m2 := &messageSomeIP{
			M:            m,
			ServiceID:    binary.BigEndian.Uint16(m.Body[0:2]),
			MethodID:     binary.BigEndian.Uint16(m.Body[2:4]),
			ClientID:     binary.BigEndian.Uint16(m.Body[8:10]),
			SessionID:    binary.BigEndian.Uint16(m.Body[10:12]),
			ProtoVersion: m.Body[12],
			InfVersion:   m.Body[13],
			MsgType:      (proto_someip.MsgTypeCode)(m.Body[14]),
			RtnCode:      (proto_someip.ErrCodeSomeIP)(m.Body[15]),
		}
		m2.Payload = m.Body[16:]

		s.Lock()
		recvQ := s.recvQ
		sizeQ := s.sizeQ
		s.Unlock()

		select {
		case recvQ <- m2:
			continue
		case <-sizeQ:
			continue
		case <-p.closeQ:
			m.Free()
			break outer
		}
	}
	go p.close()
}

// This is a puller, and doesn't permit for priorities.  We might want
// to refactor this to use a push based scheme later.
func (p *pipe) sender() {
outer:
	for {
		var m *protocol.Message
		select {
		case m = <-p.sendQ:
		case <-p.closeQ:
			break outer
		}

		if e := p.p.SendMsg(m); e != nil {
			break
		}
	}
	go p.close()
}

func (p *pipe) close() {
	_ = p.p.Close()
}

func (s *socket) SetOption(name string, value interface{}) error {
	switch name {

	case protocol.OptionTTL:
		if v, ok := value.(int); ok && v > 0 && v < 256 {
			s.Lock()
			s.ttl = v
			s.Unlock()
			return nil
		}
		return protocol.ErrBadValue

	case protocol.OptionRecvDeadline:
		if v, ok := value.(time.Duration); ok {
			s.Lock()
			s.recvExpire = v
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

	case protocol.OptionBestEffort:
		if v, ok := value.(bool); ok {
			s.Lock()
			s.bestEffort = v
			s.Unlock()
			return nil
		}
		return protocol.ErrBadValue

	case protocol.OptionWriteQLen:
		if v, ok := value.(int); ok && v >= 0 {
			s.Lock()
			// This does not impact pipes already connected.
			s.sendQLen = v
			s.Unlock()
			return nil
		}
		return protocol.ErrBadValue

	case protocol.OptionReadQLen:
		if v, ok := value.(int); ok && v >= 0 {
			recvQ := make(chan *messageSomeIP, v)
			sizeQ := make(chan struct{})
			s.Lock()
			s.recvQLen = v
			s.recvQ = recvQ
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
	}

	return protocol.ErrBadOption
}

func (s *socket) GetOption(option string) (interface{}, error) {
	switch option {
	case protocol.OptionRaw:
		return true, nil
	case protocol.OptionTTL:
		s.Lock()
		v := s.ttl
		s.Unlock()
		return v, nil
	case protocol.OptionRecvDeadline:
		s.Lock()
		v := s.recvExpire
		s.Unlock()
		return v, nil
	case protocol.OptionSendDeadline:
		s.Lock()
		v := s.sendExpire
		s.Unlock()
		return v, nil
	case protocol.OptionBestEffort:
		s.Lock()
		v := s.bestEffort
		s.Unlock()
		return v, nil
	case protocol.OptionWriteQLen:
		s.Lock()
		v := s.sendQLen
		s.Unlock()
		return v, nil
	case protocol.OptionReadQLen:
		s.Lock()
		v := s.recvQLen
		s.Unlock()
		return v, nil
	case protocol.OptionSomeIPSrvID:
		s.Lock()
		v := (int)(s.opts.ServiceID)
		s.Unlock()
		return v, nil
	case protocol.OptionSomeIPPV:
		s.Lock()
		v := (int)(s.opts.ProtoVersion)
		s.Unlock()
		return v, nil
	case protocol.OptionSomeIPInfPV:
		s.Lock()
		v := (int)(s.opts.InfVersion)
		s.Unlock()
		return v, nil
	}

	return nil, protocol.ErrBadOption
}

func (s *socket) Close() error {
	s.Lock()
	if s.closed {
		s.Unlock()
		return protocol.ErrClosed
	}
	s.closed = true
	s.Unlock()
	close(s.closeQ)
	return nil
}

func (s *socket) AddPipe(pp protocol.Pipe) error {
	p := &pipe{
		p:      pp,
		s:      s,
		closeQ: make(chan struct{}),
		sendQ:  make(chan *protocol.Message, s.sendQLen),
	}
	pp.SetPrivate(p)
	s.Lock()
	defer s.Unlock()
	if s.closed {
		return protocol.ErrClosed
	}
	s.pipes[pp.ID()] = p
	go p.sender()
	go p.receiver()
	return nil
}

func (s *socket) RemovePipe(pp protocol.Pipe) {
	p := pp.GetPrivate().(*pipe)
	close(p.closeQ)

	s.Lock()
	delete(s.pipes, p.p.ID())
	s.Unlock()
}

func (s *socket) OpenContext() (protocol.Context, error) {
	return nil, protocol.ErrProtoOp
}

func (*socket) Info() protocol.Info {
	return protocol.Info{
		Self:     Self,
		Peer:     Peer,
		SelfName: SelfName,
		PeerName: PeerName,
	}
}

func chkMsgCommon(s *socket, m *messageSomeIP) (*protocol.Message, error) {
	var e proto_someip.ErrCodeSomeIP = proto_someip.E_OK
	if s.opts.ServiceID != m.ServiceID {
		e = proto_someip.E_UNKNOWN_SERVICE
	} else if s.opts.ProtoVersion != m.ProtoVersion {
		e = proto_someip.E_WRONG_PROTOCOL_VERSION
	} else if s.opts.InfVersion != m.InfVersion {
		e = proto_someip.E_WRONG_INTERFACE_VERSION
	}

	var err error
	if e != proto_someip.E_OK {
		err = protocol.ErrProtoOp
		m.M.Body = m.M.Body[0:proto_someip.RzvBodyBySomeIP]
		m.M.Body[14] = (byte)(proto_someip.MsgTypeErr)
		m.M.Body[15] = (byte)(e)
	}

	return m.M, err
}

// NewProtocol returns a new protocol implementation.
func NewProtocol() protocol.Protocol {
	s := &socket{
		pipes:    make(map[uint32]*pipe),
		closeQ:   make(chan struct{}),
		sizeQ:    make(chan struct{}),
		recvQ:    make(chan *messageSomeIP, defaultQLen),
		sendQLen: defaultQLen,
		recvQLen: defaultQLen,
		ttl:      8,
		chkMsg:   chkMsgCommon,
	}
	return s
}

// NewSocket allocates a new Socket using the REP protocol.
func NewSocket() (protocol.Socket, error) {
	return protocol.MakeSocket(NewProtocol()), nil
}
