package proto_someip

import "go.nanomsg.org/mangos/v3/protocol"

const RzvHdrBySomeIP = 4
const RzvBodyBySomeIP = 16
const RzvBodyWithoutLth = 8
const defaultQLen = 128

// Proto specific constants
const (
	Proto_SomeIP_Req = 0x01
	Proto_SomeIP_Res = 0x02
)

type SomeIPOpts struct {
	ServiceID    uint16
	ClientID     uint16
	ProtoVersion uint8
	InfVersion   uint8
}

// MsgTypeCode code
type MsgTypeCode uint8

// SIP_RPC_684
const (
	MT_REQUEST               MsgTypeCode = 0x00
	MT_REQUEST_NO_RETURN     MsgTypeCode = 0x01
	MT_NOTIFICATION          MsgTypeCode = 0x02
	MT_REQUEST_ACK           MsgTypeCode = 0x40
	MT_REQUEST_NO_RETURN_ACK MsgTypeCode = 0x41
	MT_NOTIFICATION_ACK      MsgTypeCode = 0x42
	MT_RESPONSE              MsgTypeCode = 0x80
	MT_ERROR                 MsgTypeCode = 0x81
	MT_RESPONSE_ACK          MsgTypeCode = 0xC0
	MT_ERROR_ACK             MsgTypeCode = 0xC1
	MT_UNKNOWN               MsgTypeCode = 0xFF	
	MT_TP_REQUEST			 MsgTypeCode = 0x20
	MT_TP_REQUEST_NO_RETURN  MsgTypeCode = 0x21
	MT_TP_NOTIFICATION	     MsgTypeCode = 0x22
	MT_TP_RESPONSE			 MsgTypeCode = 0x23
	MT_TP_ERROR		         MsgTypeCode = 0x24
)

//MessageSomeIP represent constants
type MessageSomeIP struct {
	M            *protocol.Message
	ServiceID    uint16
	MethodID     uint16
	ClientID     uint16
	SessionID    uint16
	ProtoVersion uint8
	InfVersion   uint8
	MsgType      MsgTypeCode
	RtnCode      ErrCodeSomeIP
	Payload      []byte
}

//ErrCodeSomeIP code
type ErrCodeSomeIP byte

const (
	E_OK                      ErrCodeSomeIP = 0x00 //No error occurred
	E_NOT_OK                  ErrCodeSomeIP = 0x01 //An unspecified error occurred
	E_UNKNOWN_SERVICE         ErrCodeSomeIP = 0x02 //The requested Service ID is unknown.
	E_UNKNOWN_METHOD          ErrCodeSomeIP = 0x03 //The requested Method ID is unknown. Service ID is known.
	E_NOT_READY               ErrCodeSomeIP = 0x04 //Service ID and Method ID are known. Application not running.
	E_NOT_REACHABLE           ErrCodeSomeIP = 0x05 //System running the service is not reachable (internal error code only).
	E_TIMEOUT                 ErrCodeSomeIP = 0x06 //A timeout occurred (internal error code only).
	E_WRONG_PROTOCOL_VERSION  ErrCodeSomeIP = 0x07 //Version of SOME/IP protocol not supported
	E_WRONG_INTERFACE_VERSION ErrCodeSomeIP = 0x08 //Interface version mismatch
	E_MALFORMED_MESSAGE       ErrCodeSomeIP = 0x09 //Deserialization error, so that payload cannot be deserialized.
	E_WRONG_MESSAGE_TYPE      ErrCodeSomeIP = 0x0a //An unexpected message type was received (e.g.
)

//GetSomeIPBody return the raw body of someip packet
func GetSomeIPBody(m *protocol.Message) []byte {
	return m.Body[RzvBodyBySomeIP:]
}

//GetSomeIPRtnCode return error code in someip message
func GetSomeIPRtnCode(m *protocol.Message) ErrCodeSomeIP {
	return (ErrCodeSomeIP)(m.Body[15])
}

//GetSomeIPMsgType return message type in someip message
func GetSomeIPMsgType(m *protocol.Message) MsgTypeCode {
	return (MsgTypeCode)(m.Body[14])
}
