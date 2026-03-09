package main

import (
	"fmt"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type UpstreamAgent struct {
	config *UpstreamConfig
	logger *Logger

	mu        sync.Mutex
	cachedKeys []*agent.Key
	cacheTime  time.Time
}

func NewUpstreamAgent(cfg *UpstreamConfig, logger *Logger) *UpstreamAgent {
	return &UpstreamAgent{
		config: cfg,
		logger: logger,
	}
}

func (u *UpstreamAgent) connect() (agent.ExtendedAgent, net.Conn, error) {
	conn, err := net.Dial("unix", u.config.Socket)
	if err != nil {
		return nil, nil, fmt.Errorf("dial %s (%s): %w", u.config.Name, u.config.Socket, err)
	}
	return agent.NewClient(conn), conn, nil
}

func (u *UpstreamAgent) List() ([]*agent.Key, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.cachedKeys != nil {
		switch u.config.Cache {
		case CacheForever:
			u.logger.Debug("cache hit (forever)", "upstream", u.config.Name, "keys", len(u.cachedKeys))
			return u.cachedKeys, nil
		case CacheDura:
			if time.Since(u.cacheTime) < u.config.cacheDuration {
				u.logger.Debug("cache hit (duration)", "upstream", u.config.Name, "keys", len(u.cachedKeys))
				return u.cachedKeys, nil
			}
			u.logger.Debug("cache expired", "upstream", u.config.Name)
		}
	}

	a, conn, err := u.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	keys, err := a.List()
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", u.config.Name, err)
	}

	if u.config.Cache != CacheNone {
		u.cachedKeys = keys
		u.cacheTime = time.Now()
	}

	u.logger.Info("listed keys", "upstream", u.config.Name, "count", len(keys))
	return keys, nil
}

func (u *UpstreamAgent) Sign(key ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	return u.SignWithFlags(key, data, 0)
}

func (u *UpstreamAgent) SignWithFlags(key ssh.PublicKey, data []byte, flags agent.SignatureFlags) (*ssh.Signature, error) {
	a, conn, err := u.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	u.logger.Debug("sign request",
		"upstream", u.config.Name,
		"key_type", key.Type(),
		"data_len", len(data),
		"flags", flags,
	)

	sig, err := a.SignWithFlags(key, data, flags)
	if err != nil {
		return nil, fmt.Errorf("sign %s: %w", u.config.Name, err)
	}
	return sig, nil
}

func (u *UpstreamAgent) InvalidateCache() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.cachedKeys = nil
	u.cacheTime = time.Time{}
	u.logger.Info("cache invalidated", "upstream", u.config.Name)
}
