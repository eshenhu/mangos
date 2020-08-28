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

package xreq_someip

import (
	"sync"

	"testing"
	"time"

	"go.nanomsg.org/mangos/v3"
	. "go.nanomsg.org/mangos/v3/internal/test"
	"go.nanomsg.org/mangos/v3/protocol"
	"go.nanomsg.org/mangos/v3/protocol/proto_someip"
	"go.nanomsg.org/mangos/v3/protocol/xrep_someip"
	_ "go.nanomsg.org/mangos/v3/transport/inproc"
	_ "go.nanomsg.org/mangos/v3/transport/tcp_someip"
)

func TestXReqSomeIPIdentity(t *testing.T) {
	id := MustGetInfo(t, NewSocket)
	MustBeTrue(t, id.Self == mangos.ProtoSomeIPReq)
	MustBeTrue(t, id.SelfName == "someip-req")
	MustBeTrue(t, id.Peer == mangos.ProtoSomeIPRep)
	MustBeTrue(t, id.PeerName == "someip-rep")
}

func TestXReqSomeIPRaw(t *testing.T) {
	VerifyRaw(t, NewSocket)
}

func TestXReqSomeIPClosed(t *testing.T) {
	VerifyClosedRecv(t, NewSocket)
	VerifyClosedSend(t, NewSocket)
	VerifyClosedClose(t, NewSocket)
	VerifyClosedDial(t, NewSocket)
	VerifyClosedListen(t, NewSocket)
	VerifyClosedAddPipe(t, NewSocket)
}

func TestXReqSomeIPOptions(t *testing.T) {
	VerifyInvalidOption(t, NewSocket)
	VerifyOptionDuration(t, NewSocket, mangos.OptionRecvDeadline)
	VerifyOptionDuration(t, NewSocket, mangos.OptionSendDeadline)
	VerifyOptionInt(t, NewSocket, mangos.OptionReadQLen)
	VerifyOptionInt(t, NewSocket, mangos.OptionWriteQLen)
	VerifyOptionUInt16(t, NewSocket, mangos.OptionSomeIPSrvID)
}

//ToDo: should we support write WriteQLen?
func TestXReqSomeIPFromCtxMaster(t *testing.T) {
	timeout := time.Millisecond
	m := proto_someip.NewReqMsg(0x01, []byte{'0', '1', '2', '3'})

	s := GetSocket(t, NewSocket)
	ctx0, err := s.GetOption(mangos.OptionSomeIPCtxMaster)
	MustSucceed(t, err)
	ctx := ctx0.(*context)

	MustSucceed(t, s.SetOption(mangos.OptionWriteQLen, 0))
	MustSucceed(t, s.SetOption(mangos.OptionSendDeadline, timeout))
	MustSucceed(t, s.Listen(AddrTestTCPSomeIP()))
	MustBeError(t, ctx.SendMsg(m), mangos.ErrSendTimeout)
}

func TestXReqSomeIPPingPong(t *testing.T) {
	rSrv1, e := xrep_someip.NewSocket()
	MustSucceed(t, e)
	rClt1, e := NewSocket()
	MustSucceed(t, e)

	a1 := AddrTestTCPSomeIP()
	MustSucceed(t, rSrv1.Listen(a1))
	MustSucceed(t, rClt1.Dial(a1))

	opt := proto_someip.OptSomeIP{
		PV:    0x12,
		InfPV: 0x34,
		SrvID: 0x56,
		CltID: 0x78,
	}

	MustSucceed(t, rSrv1.SetOption(mangos.OptionRecvDeadline, time.Second))
	MustSucceed(t, rSrv1.SetOption(mangos.OptionSendDeadline, time.Second))
	MustSucceed(t, rSrv1.SetOption(mangos.OptionSomeIPSrvID, opt.SrvID))
	MustSucceed(t, rSrv1.SetOption(mangos.OptionSomeIPPV, opt.PV))
	MustSucceed(t, rSrv1.SetOption(mangos.OptionSomeIPInfPV, opt.InfPV))

	MustSucceed(t, rClt1.SetOption(mangos.OptionRecvDeadline, time.Second))
	MustSucceed(t, rClt1.SetOption(mangos.OptionSendDeadline, time.Second))
	MustSucceed(t, rClt1.SetOption(mangos.OptionSomeIPSrvID, opt.SrvID))
	MustSucceed(t, rClt1.SetOption(mangos.OptionSomeIPCltID, opt.CltID))
	MustSucceed(t, rClt1.SetOption(mangos.OptionSomeIPPV, opt.PV))
	MustSucceed(t, rClt1.SetOption(mangos.OptionSomeIPInfPV, opt.InfPV))

	m := proto_someip.NewReqMsg(0x02, []byte("PING"))

	MustSucceed(t, rClt1.SendMsg(m))
	ping, e := rSrv1.RecvMsg()

	body := proto_someip.GetSomeIPBody(ping)
	MustSucceed(t, e)
	MustBeTrue(t, string(body) == "PING")

	pong := proto_someip.NewRepMsg(ping, proto_someip.MsgTypeRep, proto_someip.E_OK, []byte("PONG"))

	MustSucceed(t, rSrv1.SendMsg(pong))
	pongR1, e := rClt1.RecvMsg()

	MustSucceed(t, e)
	errCode := proto_someip.GetSomeIPRtnCode(pongR1)
	MustBeTrue(t, errCode == proto_someip.E_OK)
	msgType := proto_someip.GetSomeIPMsgType(pongR1)
	MustBeTrue(t, msgType == proto_someip.MsgTypeRep)
	body = proto_someip.GetSomeIPBody(pongR1)
	MustBeTrue(t, string(body) == "PONG")

	ping2 := proto_someip.NewRepMsg(ping, proto_someip.MsgTypeErr, proto_someip.E_UNKNOWN_METHOD, nil)

	MustSucceed(t, rSrv1.SendMsg(ping2))
	pong, e = rClt1.RecvMsg()

	MustSucceed(t, e)
	errCode = proto_someip.GetSomeIPRtnCode(pong)
	MustBeTrue(t, errCode == proto_someip.E_UNKNOWN_METHOD)
	msgType = proto_someip.GetSomeIPMsgType(pong)
	MustBeTrue(t, msgType == proto_someip.MsgTypeErr)
	body = proto_someip.GetSomeIPBody(pong)
	MustBeTrue(t, len(body) == 0)

	_ = rSrv1.Close()
	_ = rClt1.Close()
}

func TestXReqSomeIPNotification(t *testing.T) {
	opt := proto_someip.OptSomeIP{
		PV:    0x12,
		InfPV: 0x34,
		SrvID: 0x56,
		CltID: 0x78,
	}
	self, e := NewReqSocketWithOpts(opt)
	MustSucceed(t, e)
	mock, _ := MockConnect(t, self)
	defer self.Close()

	ctxSlave0, e := self.GetOption(mangos.OptionSomeIPCtxSlave)
	MustSucceed(t, e)
	ctxSlave := ctxSlave0.(*context)

	ctxMaster0, e := self.GetOption(mangos.OptionSomeIPCtxMaster)
	MustSucceed(t, e)
	ctxMaster := ctxMaster0.(*context)

	m := proto_someip.NewMsgRaw(0x02, opt, proto_someip.MsgTypeNotify, proto_someip.E_OK, []byte("PING"))
	MockMustSendMsg(t, mock, m, time.Second)

	_, e = ctxMaster.RecvMsg()
	MustBeError(t, e, mangos.ErrRecvTimeout)

	ping, e := ctxSlave.RecvMsg()
	body := proto_someip.GetSomeIPBody(ping)
	MustSucceed(t, e)
	MustBeTrue(t, string(body) == "PING")
}

func TestXReqSomeIPRecvDeadline(t *testing.T) {
	s, e := NewSocket()
	MustSucceed(t, e)
	e = s.SetOption(mangos.OptionRecvDeadline, time.Millisecond)
	MustSucceed(t, e)
	m, e := s.RecvMsg()
	MustFail(t, e)
	MustBeTrue(t, e == mangos.ErrRecvTimeout)
	MustBeNil(t, m)
	_ = s.Close()
}

func TestXReqSomeIPResizeSendDiscard(t *testing.T) {
	opt := proto_someip.OptSomeIP{
		PV:    0x12,
		InfPV: 0x34,
		SrvID: 0x56,
		CltID: 0x78,
	}
	self, e := NewReqSocketWithOpts(opt)
	MustSucceed(t, e)

	m := proto_someip.NewMsgRaw(0x02, opt, proto_someip.MsgTypeReq, proto_someip.E_OK, []byte("PING"))

	MustSucceed(t, self.SetOption(mangos.OptionWriteQLen, 1))
	MustSucceed(t, self.SendMsg(m))

	MustSucceed(t, self.SetOption(mangos.OptionSendDeadline, time.Millisecond*10))
	m1 := m.Dup()
	MustBeError(t, self.SendMsg(m1), mangos.ErrSendTimeout)

	MustSucceed(t, self.SetOption(mangos.OptionSendDeadline, time.Millisecond*50))
	time.AfterFunc(time.Millisecond*5, func() {
		MustSucceed(t, self.SetOption(mangos.OptionWriteQLen, 1))
	})
	m2 := m.Dup()
	MustSucceed(t, self.SendMsg(m2))
	MustSucceed(t, self.Close())
}

func TestXReqSomeIPRecvResizeDiscard(t *testing.T) {
	opt := proto_someip.OptSomeIP{
		PV:    0x12,
		InfPV: 0x34,
		SrvID: 0x56,
		CltID: 0x78,
	}
	self, e := NewReqSocketWithOpts(opt)
	MustSucceed(t, e)
	mock, _ := MockConnect(t, self)

	defer MustClose(t, self)
	MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 1))

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		for {
			m := proto_someip.NewMsgRaw(0x02, opt, proto_someip.MsgTypeRep, proto_someip.E_OK, []byte("PING"))
			e := mock.MockSendMsg(m, time.Second)
			if e != nil {
				MustBeError(t, e, mangos.ErrClosed)
				break
			}
		}
	}()
	time.Sleep(time.Millisecond * 50)
	MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 2))
	time.Sleep(time.Millisecond * 50)
	MustSucceed(t, mock.Close())
	wg.Wait()
}

func TestXReqSomeIPCloseRecv(t *testing.T) {
	opt := proto_someip.OptSomeIP{
		PV:    0x12,
		InfPV: 0x34,
		SrvID: 0x56,
		CltID: 0x78,
	}
	self, e := NewReqSocketWithOpts(opt)
	MustSucceed(t, e)
	MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 0))
	mock, pipe := MockConnect(t, self)

	m := proto_someip.NewMsgRaw(0x02, opt, proto_someip.MsgTypeRep, proto_someip.E_OK, []byte("PING"))
	MockMustSendMsg(t, mock, m, time.Second)

	time.Sleep(time.Millisecond * 10)
	MustSucceed(t, pipe.Close())
	time.Sleep(time.Millisecond * 10)
	MustSucceed(t, self.Close())
}

func TestXReqSomeIPCloseSend0(t *testing.T) {
	opt := proto_someip.OptSomeIP{
		PV:    0x12,
		InfPV: 0x34,
		SrvID: 0x56,
		CltID: 0x78,
	}
	self, e := NewReqSocketWithOpts(opt)
	MustSucceed(t, e)
	MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 0))
	_, pipe := MockConnect(t, self)
	time.Sleep(time.Millisecond * 10)
	MustSucceed(t, pipe.Close())
	time.Sleep(time.Millisecond * 10)
}

func TestXReqSomeIPCloseSend1(t *testing.T) {
	opt := proto_someip.OptSomeIP{
		PV:    0x12,
		InfPV: 0x34,
		SrvID: 0x56,
		CltID: 0x78,
	}
	self, e := NewReqSocketWithOpts(opt)
	MustSucceed(t, e)
	MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 0))
	_, _ = MockConnect(t, self)
	time.Sleep(time.Millisecond * 10)
	MustSucceed(t, self.Close())
	time.Sleep(time.Millisecond * 10)
}

func TestXReqCloseSend2(t *testing.T) {
	opt := proto_someip.OptSomeIP{
		PV:    0x12,
		InfPV: 0x34,
		SrvID: 0x56,
		CltID: 0x78,
	}
	self, e := NewReqSocketWithOpts(opt)
	MustSucceed(t, e)
	MustSucceed(t, self.SetOption(mangos.OptionWriteQLen, 2))

	_, _ = MockConnect(t, self)
	m := proto_someip.NewReqMsg(0x01, []byte{'0', '1', '2', '3'})
	MustSucceed(t, self.SendMsg(m))
	MustSucceed(t, self.SendMsg(m))

	time.Sleep(time.Millisecond * 10)
	MustSucceed(t, self.Close())
	time.Sleep(time.Millisecond * 10)
}

func NewReqSocketWithOpts(opt proto_someip.OptSomeIP) (protocol.Socket, error) {
	s, _ := NewSocket()

	s.SetOption(mangos.OptionRecvDeadline, time.Second)
	s.SetOption(mangos.OptionSendDeadline, time.Second)
	s.SetOption(mangos.OptionSomeIPSrvID, opt.SrvID)
	s.SetOption(mangos.OptionSomeIPCltID, opt.CltID)
	s.SetOption(mangos.OptionSomeIPPV, opt.PV)
	s.SetOption(mangos.OptionSomeIPInfPV, opt.InfPV)

	return s, nil
}
