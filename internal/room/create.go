// Package room 实现"创建房间"（TanLobbyCreate）的核心逻辑。
//
// 参考 FunAuth（github.com/Yeah114/FunAuth）的 auth.TanLobbyCreate：
// 认证后的 g79 客户端会生成一套开房所需的传输/加密凭据包，
// 包括选定的中转服务器地址（RakNet + Signaling）以及加密密钥、票据等。
package room

import (
	"context"
	cryptoRand "crypto/rand"
	"fmt"
	"math/rand"
	"time"

	"github.com/Yeah114/g79client"
	"github.com/Yeah114/g79client/utils"
)

// CreateResult 创建房间的结果，包含开房所需的全部凭据。
type CreateResult struct {
	UserUniqueID           uint32 `json:"user_unique_id"`
	UserPlayerName         string `json:"user_player_name"`
	RaknetServerAddress    string `json:"raknet_server_address"`
	RaknetRand             []byte `json:"raknet_rand"`
	RaknetAESRand          []byte `json:"raknet_aes_rand"`
	EncryptKeyBytes        []byte `json:"encrypt_key_bytes"`
	DecryptKeyBytes        []byte `json:"decrypt_key_bytes"`
	SignalingServerAddress string `json:"signaling_server_address"`
	SignalingSeed          []byte `json:"signaling_seed"`
	SignalingTicket        []byte `json:"signaling_ticket"`
}

// Create 生成开房所需的中转信息与加密凭据。
// cli 必须是已完成认证（UserToken 非空）的 g79 客户端。
func Create(ctx context.Context, cli *g79client.Client) (*CreateResult, error) {
	if cli == nil {
		return nil, fmt.Errorf("Create: nil client")
	}
	if cli.UserToken == "" {
		return nil, fmt.Errorf("Create: missing user token")
	}
	if cli.UserDetail == nil {
		detail, err := cli.GetUserDetail()
		if err != nil {
			return nil, fmt.Errorf("Create: GetUserDetail: %w", err)
		}
		cli.UserDetail = &detail.Entity
	}

	raknetAddr, signalingAddr, err := selectTransferServer(cli)
	if err != nil {
		return nil, fmt.Errorf("Create: %w", err)
	}

	encryptedToken := utils.GetEncryptedToken(cli.UserToken)

	raknetRand := make([]byte, 16)
	if _, err = cryptoRand.Read(raknetRand); err != nil {
		return nil, fmt.Errorf("Create: rand read: %w", err)
	}

	raknetAESRand, err := utils.AesECBEncrypt(raknetRand, encryptedToken)
	if err != nil {
		return nil, fmt.Errorf("Create: aes encrypt: %w", err)
	}
	if len(raknetAESRand) >= 16 {
		raknetAESRand = raknetAESRand[:16]
	}

	encryptKeyBytes := append(append(make([]byte, 0, len(encryptedToken)+len(raknetRand)), encryptedToken...), raknetRand...)
	decryptKeyBytes := append(append(make([]byte, 0, len(encryptedToken)+len(raknetRand)), raknetRand...), encryptedToken...)

	signalingSeed := make([]byte, 16)
	if _, err = cryptoRand.Read(signalingSeed); err != nil {
		return nil, fmt.Errorf("Create: rand read: %w", err)
	}

	signalingTicket, err := utils.AesECBEncrypt(signalingSeed, []byte(cli.UserToken))
	if err != nil {
		return nil, fmt.Errorf("Create: aes encrypt: %w", err)
	}
	if len(signalingTicket) >= 16 {
		signalingTicket = signalingTicket[:16]
	}

	uid, err := cli.GetUserIDInt()
	if err != nil {
		return nil, fmt.Errorf("Create: parse user id: %w", err)
	}

	playerName := cli.UserID
	if cli.UserDetail != nil && cli.UserDetail.Name != "" {
		playerName = cli.UserDetail.Name
	}

	return &CreateResult{
		UserUniqueID:           uint32(uid),
		UserPlayerName:         playerName,
		RaknetServerAddress:    raknetAddr,
		RaknetRand:             raknetRand,
		RaknetAESRand:          raknetAESRand,
		EncryptKeyBytes:        encryptKeyBytes,
		DecryptKeyBytes:        decryptKeyBytes,
		SignalingServerAddress: signalingAddr,
		SignalingSeed:          signalingSeed,
		SignalingTicket:        signalingTicket,
	}, nil
}

type transferServerEntry struct {
	Status         int    `json:"status"`
	ServerIP       string `json:"ip"`
	SignalWebPort  int    `json:"SignalWebPort"`
	WebsocketPorts []int  `json:"ports"`
}

// selectTransferServer 从全局中转服务器列表中随机挑选一台可用服务器，
// 返回 RakNet 地址与 Signaling 信令地址。
func selectTransferServer(cli *g79client.Client) (string, string, error) {
	servers, err := g79client.GetGlobalG79TransferServers()
	if err != nil {
		return "", "", fmt.Errorf("SelectTransferServer: %w", err)
	}

	var list []transferServerEntry
	for _, s := range servers {
		list = append(list, transferServerEntry{
			Status:         int(s.Status.Int64()),
			ServerIP:       s.IP,
			SignalWebPort:  int(s.SignalWebPort.Int64()),
			WebsocketPorts: s.Ports,
		})
	}
	available := make([]transferServerEntry, 0, len(list))
	for _, entry := range list {
		if len(entry.WebsocketPorts) == 0 || entry.ServerIP == "" || entry.SignalWebPort == 0 {
			continue
		}
		available = append(available, entry)
	}
	if len(available) == 0 {
		return "", "", fmt.Errorf("SelectTransferServer: no available server")
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	selected := available[rng.Intn(len(available))]
	port := selected.WebsocketPorts[rng.Intn(len(selected.WebsocketPorts))]

	raknetAddr := fmt.Sprintf("%s:%d", selected.ServerIP, port)
	signalingAddr := fmt.Sprintf("%s:%d", selected.ServerIP, selected.SignalWebPort)

	return raknetAddr, signalingAddr, nil
}
