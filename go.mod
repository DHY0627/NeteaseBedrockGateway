module NeteaseBedrockGateway

go 1.25

require (
	github.com/Happy2018new/nemc-tan-lobby-solver v0.0.0-00010101000000-000000000000
	github.com/Yeah114/g79client v0.0.0-00010101000000-000000000000
	github.com/pion/dtls/v3 v3.0.7
	github.com/pion/logging v0.2.4
	github.com/sandertv/go-raknet v1.15.1
)

require github.com/database64128/chacha8-go v0.0.0-20250815115417-e0f2726d8bd0 // indirect

require (
	github.com/coder/websocket v1.8.14 // indirect
	github.com/df-mc/atomic v1.10.0 // indirect
	github.com/go-gl/mathgl v1.2.0 // indirect
	github.com/go-jose/go-jose/v3 v3.0.4 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/muhammadmuzzammil1998/jsonc v1.0.0 // indirect
	github.com/pion/ice/v4 v4.0.10 // indirect
	github.com/pion/interceptor v0.1.40 // indirect
	github.com/pion/mdns/v2 v2.0.7 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.15 // indirect
	github.com/pion/rtp v1.8.21 // indirect
	github.com/pion/sdp/v3 v3.0.15 // indirect
	github.com/pion/srtp/v3 v3.0.7 // indirect
	github.com/pion/stun/v3 v3.0.0 // indirect
	github.com/pion/transport/v3 v3.0.7 // indirect
	github.com/pion/turn/v4 v4.1.1 // indirect
	github.com/ugorji/go/codec v1.3.0 // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	golang.org/x/crypto v0.39.0 // indirect
	golang.org/x/net v0.41.0 // indirect
	golang.org/x/sys v0.36.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)

replace github.com/Yeah114/g79client => ../FunAuth/modules/g79client

replace github.com/Happy2018new/nemc-tan-lobby-solver => github.com/DHY0627/nemc-tan-lobby-solver v0.0.0-20261006154354-f2649a16ba10

replace github.com/sandertv/go-raknet => ../go-raknet-netease
