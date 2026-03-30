package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"douyin-live-record/internal/model"
	"douyin-live-record/internal/storage"

	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	store *storage.Store
	ttl   time.Duration
}

func NewService(store *storage.Store, ttl time.Duration) *Service {
	return &Service{store: store, ttl: ttl}
}

func (s *Service) EnsureAdmin(ctx context.Context, username, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.store.EnsureAdmin(ctx, username, string(hash))
}

func (s *Service) Login(ctx context.Context, username, password string) (string, error) {
	if err := s.store.DeleteExpiredSessions(ctx); err != nil {
		return "", err
	}

	user, err := s.store.GetUserByUsername(ctx, username)
	if err != nil {
		return "", err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", errors.New("invalid username or password")
	}

	token, err := randomToken()
	if err != nil {
		return "", err
	}
	if err := s.store.CreateSession(ctx, user.ID, token, time.Now().UTC().Add(s.ttl)); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (model.Session, error) {
	if token == "" {
		return model.Session{}, errors.New("missing session token")
	}

	session, err := s.store.GetSessionByToken(ctx, token)
	if err != nil {
		return model.Session{}, err
	}
	if session.ExpiresAt.Before(time.Now().UTC()) {
		_ = s.store.DeleteSession(ctx, token)
		return model.Session{}, errors.New("session expired")
	}
	if err := s.store.TouchSession(ctx, token); err != nil {
		return model.Session{}, err
	}
	return session, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.store.DeleteSession(ctx, token)
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
