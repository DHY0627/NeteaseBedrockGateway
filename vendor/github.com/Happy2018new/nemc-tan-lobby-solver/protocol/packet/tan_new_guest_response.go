package packet

import "github.com/Happy2018new/nemc-tan-lobby-solver/protocol/encoding"

// TanNewGuestResponse is sent by the transfer server to the room host
// when a new player enters the room. ErrorCode 0 = success.
type TanNewGuestResponse struct {
	ErrorCode    int8
	PlayerIDList []uint32
}

func (*TanNewGuestResponse) ID() uint16 {
	return IDTanNewGuestResponse
}

func (*TanNewGuestResponse) BoundType() uint8 {
	return BoundTypeClient
}

func (t *TanNewGuestResponse) Marshal(io encoding.IO) {
	io.Int8(&t.ErrorCode)
	if t.ErrorCode != 0 {
		return
	}
	encoding.FuncSliceUint8Length(io, &t.PlayerIDList, io.Uint32)
}
