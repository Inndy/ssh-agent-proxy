package main

import (
	"errors"
	"fmt"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/sync/errgroup"
)

var errNotSupported = errors.New("operation not supported by proxy")

type ProxyAgent struct {
	upstreams []*UpstreamAgent
	logger    *Logger

	mu     sync.RWMutex
	keyMap map[string]*UpstreamAgent
}

func NewProxyAgent(upstreams []*UpstreamAgent, logger *Logger) *ProxyAgent {
	return &ProxyAgent{
		upstreams: upstreams,
		logger:    logger,
		keyMap:    make(map[string]*UpstreamAgent),
	}
}

func (p *ProxyAgent) List() ([]*agent.Key, error) {
	type result struct {
		keys     []*agent.Key
		upstream *UpstreamAgent
	}

	var g errgroup.Group
	results := make([]result, len(p.upstreams))

	for i, u := range p.upstreams {
		i, u := i, u
		g.Go(func() error {
			keys, err := u.List()
			if err != nil {
				p.logger.Warn("upstream list failed", "upstream", u.config.Name, "error", err)
				return nil
			}
			results[i] = result{keys: keys, upstream: u}
			return nil
		})
	}
	g.Wait()

	newKeyMap := make(map[string]*UpstreamAgent)
	var allKeys []*agent.Key

	for _, r := range results {
		if r.upstream == nil {
			continue
		}
		for _, key := range r.keys {
			keyID := string(key.Marshal())
			if existing, ok := newKeyMap[keyID]; ok {
				p.logger.Warn("duplicate key",
					"fingerprint", ssh.FingerprintSHA256(key),
					"kept_upstream", existing.config.Name,
					"skipped_upstream", r.upstream.config.Name,
				)
				continue
			}
			newKeyMap[keyID] = r.upstream
			allKeys = append(allKeys, key)
			p.logger.Debug("key available",
				"upstream", r.upstream.config.Name,
				"type", key.Type(),
				"fingerprint", ssh.FingerprintSHA256(key),
				"comment", key.Comment,
			)
		}
	}

	p.mu.Lock()
	p.keyMap = newKeyMap
	p.mu.Unlock()

	p.logger.Info("key list complete", "total_keys", len(allKeys))
	return allKeys, nil
}

func (p *ProxyAgent) lookupUpstream(key ssh.PublicKey) (*UpstreamAgent, error) {
	keyID := string(key.Marshal())

	p.mu.RLock()
	u, ok := p.keyMap[keyID]
	p.mu.RUnlock()

	if ok {
		return u, nil
	}

	p.logger.Debug("key not in map, invalidating caches and refreshing", "fingerprint", ssh.FingerprintSHA256(key))
	p.InvalidateAllCaches()
	if _, err := p.List(); err != nil {
		return nil, err
	}

	p.mu.RLock()
	u, ok = p.keyMap[keyID]
	p.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("key %s not found in any upstream", ssh.FingerprintSHA256(key))
	}
	return u, nil
}

func (p *ProxyAgent) Sign(key ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	return p.SignWithFlags(key, data, 0)
}

func (p *ProxyAgent) SignWithFlags(key ssh.PublicKey, data []byte, flags agent.SignatureFlags) (*ssh.Signature, error) {
	u, err := p.lookupUpstream(key)
	if err != nil {
		return nil, err
	}

	p.logger.Info("routing sign request",
		"upstream", u.config.Name,
		"key_fingerprint", ssh.FingerprintSHA256(key),
		"data_len", len(data),
		"flags", flags,
	)

	sig, err := u.SignWithFlags(key, data, flags)
	if err != nil {
		return nil, err
	}

	p.logger.Info("sign success", "upstream", u.config.Name, "key_fingerprint", ssh.FingerprintSHA256(key))
	return sig, nil
}

func (p *ProxyAgent) Extension(extensionType string, contents []byte) ([]byte, error) {
	return nil, agent.ErrExtensionUnsupported
}

func (p *ProxyAgent) Add(key agent.AddedKey) error          { return errNotSupported }
func (p *ProxyAgent) Remove(key ssh.PublicKey) error         { return errNotSupported }
func (p *ProxyAgent) RemoveAll() error                       { return errNotSupported }
func (p *ProxyAgent) Lock(passphrase []byte) error           { return errNotSupported }
func (p *ProxyAgent) Unlock(passphrase []byte) error         { return errNotSupported }
func (p *ProxyAgent) Signers() ([]ssh.Signer, error)        { return nil, errNotSupported }

func (p *ProxyAgent) InvalidateAllCaches() {
	for _, u := range p.upstreams {
		u.InvalidateCache()
	}
	p.logger.Info("all caches invalidated")
}
