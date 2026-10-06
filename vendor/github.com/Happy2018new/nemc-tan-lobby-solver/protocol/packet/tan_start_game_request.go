package packet

import "github.com/Happy2018new/nemc-tan-lobby-solver/protocol/encoding"

// TanStartGameRequest is sent by the room host to the transfer server
// after the host is ready to start the game. The transfer server then
// broadcasts a TanNotifyServerReady to every player in the room so they
// know how to connect to the host (NetherNetID etc.).
//
// RoomID identifies the room being started; the remaining fields mirror
// TanNotifyServerReady exactly: the transfer server relays them unchanged
// to players as the "server ready" notification.
type TanStartGameRequest struct {
	RoomID                uint32
	ServerAddress         string
	ServerRaknetGuid      string
	RTCRoomID             string
	NetherNetID           string
	WebRTCCompressEnabled bool
}

func (*TanStartGameRequest) ID() uint16 {
	return IDTanStartGameRequest
}

func (*TanStartGameRequest) BoundType() uint8 {
	return BoundTypeServer
}

func (t *TanStartGameRequest) Marshal(io encoding.IO) {
	io.Uint32(&t.RoomID)
	io.StringUTF(&t.ServerAddress)
	io.StringUTF(&t.ServerRaknetGuid)
	io.StringUTF(&t.RTCRoomID)
	io.StringUTF(&t.NetherNetID)
	io.Bool(&t.WebRTCCompressEnabled)
}
