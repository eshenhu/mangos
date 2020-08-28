package proto_someip

import (
	"encoding/binary"


	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/protocol"
)

type OptSomeIP struct {
	PV    byte
	InfPV byte
	SrvID uint16
	CltID uint16
}

func NewSomeIPMsg(l int) *protocol.Message {
	msg := mangos.NewMessage(l + RzvBodyBySomeIP)
	msg.Header = msg.Header[0:RzvHdrBySomeIP]
	msg.Body = msg.Body[:l+RzvBodyBySomeIP]
	return msg
}

func NewReqMsg(method uint16, snd []byte) *protocol.Message {
	msg := NewSomeIPMsg(len(snd))
	copy(msg.Body[RzvBodyBySomeIP:], snd[:])
	binary.BigEndian.PutUint16(msg.Body[2:4], method)
	msg.Body[14] = (byte)(MsgTypeReq)
	msg.Body[15] = 0
	return msg
}

func NewSomeIPRepMsg(l int) *protocol.Message {
	msg := mangos.NewMessage(l + RzvBodyBySomeIP)
	msg.Header = msg.Header[0 : 4+RzvHdrBySomeIP]
	msg.Body = msg.Body[:l+RzvBodyBySomeIP]
	return msg
}

type msgTypeCode = MsgTypeCode
type errCode = ErrCodeSomeIP

func NewMsgRaw(method uint16, opt OptSomeIP, t MsgTypeCode, c ErrCodeSomeIP, snd []byte) *protocol.Message {
	msg := NewSomeIPMsg(len(snd))
	copy(msg.Body[RzvBodyBySomeIP:], snd[:])

	binary.BigEndian.PutUint16(msg.Body[0:2], opt.SrvID)
	binary.BigEndian.PutUint16(msg.Body[2:4], method)
	binary.BigEndian.PutUint16(msg.Body[8:10], opt.CltID)
	msg.Body[12] = opt.PV
	msg.Body[13] = opt.InfPV
	msg.Body[14] = (byte)(t)
	msg.Body[15] = (byte)(c)
	return msg
}

// build Respond Msg based on the Request Msg
// copy pipeId, serviceId+methodId, clientId+sessionId, protoVersion+infVersion
func NewRepMsg(m *protocol.Message, msgType msgTypeCode, rtnCode errCode, snd []byte) *protocol.Message {
	m0 := m.Dup()
	//m0.Header = m0.Header[0:4]
	m0.Body = m0.Body[0:RzvBodyBySomeIP]
	m0.Body[14] = (byte)(msgType)
	m0.Body[15] = (byte)(rtnCode)
	m0.Body = append(m0.Body, snd...)
	return m0
}