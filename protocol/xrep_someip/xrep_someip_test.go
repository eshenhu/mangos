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

package xrep_someip

import (
	"encoding/binary"
	"testing"
	"time"

	"go.nanomsg.org/mangos/v3"
	. "go.nanomsg.org/mangos/v3/internal/test"
	"go.nanomsg.org/mangos/v3/protocol"
	_ "go.nanomsg.org/mangos/v3/protocol"
	"go.nanomsg.org/mangos/v3/protocol/proto_someip"
	_ "go.nanomsg.org/mangos/v3/protocol/proto_someip"
	_ "go.nanomsg.org/mangos/v3/protocol/xreq_someip"
	_ "go.nanomsg.org/mangos/v3/transport/inproc"
	_ "go.nanomsg.org/mangos/v3/transport/tcp_someip"
)

type OptSomeIP struct {
	PV    byte
	InfPV byte
	SrvID uint16
	CltID uint16
}

func TestXRepSomeIPIdentity(t *testing.T) {
	id := MustGetInfo(t, NewSocket)
	MustBeTrue(t, id.Self == mangos.ProtoSomeIPRep)
	MustBeTrue(t, id.SelfName == "someip-rep")
	MustBeTrue(t, id.Peer == mangos.ProtoSomeIPReq)
	MustBeTrue(t, id.PeerName == "someip-req")
}

func TestXRepClosed(t *testing.T) {
	VerifyClosedRecv(t, NewSocket)
	VerifyClosedSend(t, NewSocket)
	VerifyClosedClose(t, NewSocket)
	VerifyClosedDial(t, NewSocket)
	VerifyClosedListen(t, NewSocket)
	VerifyClosedAddPipe(t, NewSocket)
}

func TestXRepOptions(t *testing.T) {
	VerifyInvalidOption(t, NewSocket)
	VerifyOptionDuration(t, NewSocket, mangos.OptionRecvDeadline)
	VerifyOptionDuration(t, NewSocket, mangos.OptionSendDeadline)
	VerifyOptionInt(t, NewSocket, mangos.OptionReadQLen)
	VerifyOptionInt(t, NewSocket, mangos.OptionWriteQLen)
	VerifyOptionBool(t, NewSocket, mangos.OptionBestEffort)
	VerifyOptionTTL(t, NewSocket)
}

func TestXRepNoHeader(t *testing.T) {
	opt := proto_someip.OptSomeIP{
		PV:    0x12,
		InfPV: 0x34,
		SrvID: 0x56,
		CltID: 0x78,
	}
	self, e := NewRepSocketWithOpts(opt)
	MustSucceed(t, e)

	m := proto_someip.NewMsgRaw(0x02, opt, proto_someip.MsgTypeRep, proto_someip.E_OK, []byte("PING"))
	e = self.SendMsg(m)
	MustSucceed(t, e)

	MustClose(t, self)
}

func TestXRepRecvDeadline(t *testing.T) {
	opt := proto_someip.OptSomeIP{
		PV:    0x12,
		InfPV: 0x34,
		SrvID: 0x56,
		CltID: 0x78,
	}
	self, e := NewRepSocketWithOpts(opt)
	MustSucceed(t, e)

	MustSucceed(t, self.SetOption(mangos.OptionRecvDeadline, time.Millisecond))
	MustNotRecv(t, self, mangos.ErrRecvTimeout)
	MustClose(t, self)
}

/*
func newRequest(id uint32, content string) *mangos.Message {
	m := mangos.NewMessage(len(content) + 8)
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b[:4], id|0x80000000)
	// Requests (coming in) will be entirely on the body.
	m.Body = append(m.Body, b...)
	m.Body = append(m.Body, []byte(content)...)
	return m
}

func newReply(id uint32, p mangos.Pipe, content string) *mangos.Message {
	m := mangos.NewMessage(len(content))
	b := make([]byte, 8)
	binary.BigEndian.PutUint32(b, p.ID())            // outgoing pipe ID
	binary.BigEndian.PutUint32(b[4:], id|0x80000000) // request ID
	m.Header = append(m.Header, b...)
	m.Body = append(m.Body, []byte(content)...)
	return m
}
*/

func newRequest(id uint16, content string) *mangos.Message {
	return proto_someip.NewReqMsg(id, []byte(content))
}

func newRequestRaw(id uint16, opt proto_someip.OptSomeIP, content string) *mangos.Message {
	return proto_someip.NewMsgRaw(id, opt, proto_someip.MsgTypeReq, proto_someip.E_OK, []byte(content))
}

func newReply(id uint16, p mangos.Pipe, opt proto_someip.OptSomeIP, content string) *mangos.Message {
	m := proto_someip.NewMsgRaw(id, opt, proto_someip.MsgTypeRep, proto_someip.E_OK, []byte(content))
	m.Header = m.Header[0:4]
	binary.BigEndian.PutUint32(m.Header[0:4], p.ID()) // outgoing pipe ID
	return m
}

func newNotify(id uint16, p mangos.Pipe, opt proto_someip.OptSomeIP, content string) *mangos.Message {
	m := proto_someip.NewMsgRaw(id, opt, proto_someip.MsgTypeNotify, proto_someip.E_OK, []byte(content))
	m.Header = m.Header[0:4]
	binary.BigEndian.PutUint32(m.Header[0:4], p.ID()) // outgoing pipe ID
	return m
}

func TestXRepSendTimeout(t *testing.T) {
	opt := proto_someip.OptSomeIP{
		PV:    0x12,
		InfPV: 0x34,
		SrvID: 0x56,
		CltID: 0x78,
	}
	self, e := NewRepSocketWithOpts(opt)
	MustSucceed(t, e)

	timeout := time.Millisecond * 10
	MustSucceed(t, self.SetOption(mangos.OptionWriteQLen, 0))
	MustSucceed(t, self.SetOption(mangos.OptionSendDeadline, timeout))

	_, p := MockConnect(t, self)
	MustSendMsg(t, self, newReply(0, p, opt, "zero"))
	MustBeError(t, self.SendMsg(newReply(1, p, opt, "one")), mangos.ErrSendTimeout)
	MustClose(t, self)
}
func TestXRepSendError(t *testing.T) {
	opt := proto_someip.OptSomeIP{
		PV:    0x12,
		InfPV: 0x34,
		SrvID: 0x56,
		CltID: 0x78,
	}
	self, e := NewRepSocketWithOpts(opt)
	MustSucceed(t, e)

	mp, _ := MockConnect(t, self)

	m := newRequestRaw(1, opt, "hello")
	m.Body[12] = 0x21 //PV error
	m.Body[13] = 0x43 //InfPV error
	MockMustSendMsg(t, mp, m, time.Second)

	time.Sleep(time.Millisecond * 50)

	m0, e := self.RecvMsg()
	MustBeNil(t, m0)
	MustBeError(t, e, mangos.ErrProtoOp)

	m1, e := MockRecvMsg(t, mp, time.Second)
	MustSucceed(t, e)
	MustBeTrue(t, len(m1.Body) == proto_someip.RzvBodyBySomeIP)
	MustBeTrue(t, (proto_someip.GetSomeIPMsgType(m1) == proto_someip.MsgTypeErr))
	MustBeTrue(t, (proto_someip.GetSomeIPRtnCode(m1) == proto_someip.E_WRONG_PROTOCOL_VERSION))

	MustClose(t, self)
}

func TestXRepSendBestEffort(t *testing.T) {
	opt := proto_someip.OptSomeIP{
		PV:    0x12,
		InfPV: 0x34,
		SrvID: 0x56,
		CltID: 0x78,
	}
	self, e := NewRepSocketWithOpts(opt)
	MustSucceed(t, e)

	timeout := time.Millisecond * 10

	MustSucceed(t, self.SetOption(mangos.OptionWriteQLen, 0))
	MustSucceed(t, self.SetOption(mangos.OptionSendDeadline, timeout))
	MustSucceed(t, self.SetOption(mangos.OptionBestEffort, true))

	_, p := MockConnect(t, self)
	for i := 0; i < 100; i++ {
		MustSendMsg(t, self, newReply(0, p, opt, ""))
	}
	MustClose(t, self)
}

func TestXRepPipeCloseAbort(t *testing.T) {
	opt := proto_someip.OptSomeIP{
		PV:    0x12,
		InfPV: 0x34,
		SrvID: 0x56,
		CltID: 0x78,
	}
	self, e := NewRepSocketWithOpts(opt)
	MustSucceed(t, e)

	MustSucceed(t, self.SetOption(mangos.OptionWriteQLen, 0))
	MustSucceed(t, self.SetOption(mangos.OptionSendDeadline, time.Second))

	_, p := MockConnect(t, self)
	time.AfterFunc(time.Millisecond*20, func() {
		MustSucceed(t, p.Close())
	})
	MustSendMsg(t, self, newReply(0, p, opt, "good"))
	MustBeError(t, self.SendMsg(newReply(1, p, opt, "bad")), mangos.ErrClosed)
	MustClose(t, self)
}

func TestXRepRecvCloseAbort(t *testing.T) {
	opt := proto_someip.OptSomeIP{
		PV:    0x12,
		InfPV: 0x34,
		SrvID: 0x56,
		CltID: 0x78,
	}
	self, e := NewRepSocketWithOpts(opt)
	MustSucceed(t, e)

	MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 1))
	MustSucceed(t, self.SetOption(mangos.OptionRecvDeadline, time.Millisecond*10))

	mp, p := MockConnect(t, self)
	MockMustSendMsg(t, mp, newRequest(1, "one"), time.Second)
	MockMustSendMsg(t, mp, newRequest(2, "two"), time.Second)

	time.Sleep(time.Millisecond * 10)
	MustSucceed(t, p.Close())
	MustClose(t, self)
}

func TestXRepResizeRecv1(t *testing.T) {
	opt := proto_someip.OptSomeIP{
		PV:    0x12,
		InfPV: 0x34,
		SrvID: 0x56,
		CltID: 0x78,
	}
	self, e := NewRepSocketWithOpts(opt)
	MustSucceed(t, e)

	mp, _ := MockConnect(t, self)

	MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 0))
	MustSucceed(t, self.SetOption(mangos.OptionRecvDeadline, time.Millisecond))
	MockMustSendMsg(t, mp, newRequest(1, "hello"), time.Second)

	time.Sleep(time.Millisecond * 50)
	MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 2))
	MustNotRecv(t, self, mangos.ErrRecvTimeout)
	MustClose(t, self)
}

func TestXRepResizeRecv2(t *testing.T) {
	opt := proto_someip.OptSomeIP{
		PV:    0x12,
		InfPV: 0x34,
		SrvID: 0x56,
		CltID: 0x78,
	}
	self, e := NewRepSocketWithOpts(opt)
	MustSucceed(t, e)

	mp, p := MockConnect(t, self)

	MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 1))
	MustSucceed(t, self.SetOption(mangos.OptionRecvDeadline, time.Second))

	time.AfterFunc(time.Millisecond*50, func() {
		MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 2))
		MockMustSendMsg(t, mp, newReply(1, p, opt, "one"), time.Second)
	})
	m := MustRecvMsg(t, self)
	MustBeTrue(t, string(proto_someip.GetSomeIPBody(m)) == "one")
	MustClose(t, self)
}

func NewRepSocketWithOpts(opt proto_someip.OptSomeIP) (protocol.Socket, error) {
	s, _ := NewSocket()

	s.SetOption(mangos.OptionRecvDeadline, time.Second)
	s.SetOption(mangos.OptionSendDeadline, time.Second)
	s.SetOption(mangos.OptionSomeIPSrvID, opt.SrvID)
	s.SetOption(mangos.OptionSomeIPCltID, opt.CltID)
	s.SetOption(mangos.OptionSomeIPPV, opt.PV)
	s.SetOption(mangos.OptionSomeIPInfPV, opt.InfPV)

	return s, nil
}
