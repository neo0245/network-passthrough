package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"
)

type TokenRecord struct {
	ID        string    `json:"id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
}

type Validator interface {
	Validate(token string, nonce [12]byte) error
	Revoke(id string) error
	Generate(ttl time.Duration) (TokenRecord, error)
	List() []TokenRecord
}

type Store struct {
	mu     sync.Mutex
	path   string
	tokens map[string]TokenRecord
	replay map[string]time.Time
}

func NewStore(path string) (*Store, error) {
	s := &Store{
		path:   path,
		tokens: make(map[string]TokenRecord),
		replay: make(map[string]time.Time),
	}
	if path == "" {
		return s, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	var list []TokenRecord
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	for _, rec := range list {
		s.tokens[rec.ID] = rec
	}
	return s, nil
}

func (s *Store) Validate(token string, nonce [12]byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var match *TokenRecord
	for _, rec := range s.tokens {
		if rec.Token == token {
			r := rec
			match = &r
			break
		}
	}
	if match == nil {
		return errors.New("invalid token")
	}
	if match.Revoked || now.After(match.ExpiresAt) {
		return errors.New("token expired or revoked")
	}
	replayKey := token + ":" + base64.RawStdEncoding.EncodeToString(nonce[:])
	if expiry, ok := s.replay[replayKey]; ok && expiry.After(now) {
		return errors.New("replayed nonce")
	}
	s.replay[replayKey] = now.Add(time.Until(match.ExpiresAt))
	return nil
}

func (s *Store) Revoke(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.tokens[id]
	if !ok {
		return errors.New("token not found")
	}
	rec.Revoked = true
	s.tokens[id] = rec
	return s.flushLocked()
}

func (s *Store) Generate(ttl time.Duration) (TokenRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return TokenRecord{}, err
	}
	idBytes := make([]byte, 8)
	if _, err := rand.Read(idBytes); err != nil {
		return TokenRecord{}, err
	}
	rec := TokenRecord{
		ID:        base64.RawURLEncoding.EncodeToString(idBytes),
		Token:     base64.RawURLEncoding.EncodeToString(tokenBytes),
		ExpiresAt: time.Now().Add(ttl),
	}
	s.tokens[rec.ID] = rec
	return rec, s.flushLocked()
}

func (s *Store) List() []TokenRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]TokenRecord, 0, len(s.tokens))
	for _, rec := range s.tokens {
		out = append(out, rec)
	}
	return out
}

func (s *Store) flushLocked() error {
	if s.path == "" {
		return nil
	}
	list := make([]TokenRecord, 0, len(s.tokens))
	for _, rec := range s.tokens {
		list = append(list, rec)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
