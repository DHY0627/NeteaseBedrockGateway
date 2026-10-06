package message

import (
	"bytes"
	"encoding/binary"
)

type OpenConnectionReply1 struct {
	Magic                  [16]byte
	ServerGUID             int64
	Secure                 bool
	Cookie                 []byte // cloudburst/Geyser Secure 模式下的 4 字节安全 Cookie
	ServerPreferredMTUSize uint16
}

func (pk *OpenConnectionReply1) Write(buf *bytes.Buffer) {
	_ = binary.Write(buf, binary.BigEndian, IDOpenConnectionReply1)
	_ = binary.Write(buf, binary.BigEndian, unconnectedMessageSequence)
	_ = binary.Write(buf, binary.BigEndian, pk.ServerGUID)
	_ = binary.Write(buf, binary.BigEndian, pk.Secure)
	if pk.Secure && len(pk.Cookie) > 0 {
		buf.Write(pk.Cookie)
	}
	_ = binary.Write(buf, binary.BigEndian, pk.ServerPreferredMTUSize)
}

func (pk *OpenConnectionReply1) Read(buf *bytes.Buffer) error {
	_ = binary.Read(buf, binary.BigEndian, &pk.Magic)
	_ = binary.Read(buf, binary.BigEndian, &pk.ServerGUID)
	_ = binary.Read(buf, binary.BigEndian, &pk.Secure)
	if pk.Secure {
		// cloudburst/Geyser 在 Secure=true 时，Reply1 会附带 4 字节安全 Cookie，
		// 保存并在 Request2 中回传；否则 MTU 读取错位。
		cookie := make([]byte, 4)
		if _, err := buf.Read(cookie); err != nil {
			return err
		}
		pk.Cookie = cookie
	}
	return binary.Read(buf, binary.BigEndian, &pk.ServerPreferredMTUSize)
}
