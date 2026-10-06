package wplauncher

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// Session x19 登录会话, 持有 entity_id 与 token
// 对应 Kotlin 端 WPLauncherSession
type Session struct {
	ID   uint64
	Name string

	mu    sync.RWMutex
	token string

	client *X19Client

	heartbeatCancel context.CancelFunc

	// OnRefresh 每次心跳刷新成功后回调 (参数为新 token)
	OnRefresh func(token string)
	// OnHeartbeatError 心跳失败时回调, 之后心跳自动停止
	OnHeartbeatError func(err error)
}

func newSession(entity *AuthEntity, client *X19Client) *Session {
	return &Session{
		ID:     uint64(entity.EntityID),
		token:  entity.Token,
		client: client,
	}
}

// NewSession 从已有的 entity_id / token 恢复会话 (例如上次登录保存的结果),
// 可用于重启自动心跳而无需重新登录
func NewSession(id uint64, token string, client *http.Client) *Session {
	return &Session{ID: id, token: token, client: NewX19Client(client)}
}

// Token 返回当前 token (线程安全)
func (s *Session) Token() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.token
}

func (s *Session) setToken(t string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = t
}

// Refresh 执行一次心跳, 成功后更新 token
func (s *Session) Refresh(ctx context.Context) error {
	newToken, err := s.client.Refresh(ctx, s.ID, s.Token())
	if err != nil {
		return err
	}
	s.setToken(newToken)
	return nil
}

// GetSelfDetail 获取自身信息并更新 Session.Name, 可用于验证 token 有效性
func (s *Session) GetSelfDetail(ctx context.Context) (*SelfDetail, error) {
	detail, err := s.client.GetSelfDetail(ctx, s.ID, s.Token())
	if err != nil {
		return nil, err
	}
	s.Name = detail.Name
	return detail, nil
}

// StartHeartbeat 启动后台心跳协程, 每 interval 自动刷新 token
// 可通过返回的 cancel 或调用 StopHeartbeat 停止
func (s *Session) StartHeartbeat(interval time.Duration) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.heartbeatCancel != nil {
		s.heartbeatCancel()
	}
	s.heartbeatCancel = cancel
	s.mu.Unlock()

	go s.heartbeatLoop(ctx, interval)
	return cancel
}

// StopHeartbeat 停止后台心跳协程
func (s *Session) StopHeartbeat() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.heartbeatCancel != nil {
		s.heartbeatCancel()
		s.heartbeatCancel = nil
	}
}

func (s *Session) heartbeatLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Refresh(ctx); err != nil {
				if ctx.Err() != nil {
					return // 会话被主动停止
				}
				if s.OnHeartbeatError != nil {
					s.OnHeartbeatError(err)
				}
				return
			}
			if s.OnRefresh != nil {
				s.OnRefresh(s.Token())
			}
		}
	}
}
