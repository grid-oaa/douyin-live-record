package auth

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"douyin-live-record/internal/storage"
)

func TestLoginAndAuthenticate(t *testing.T) {
	store, err := storage.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	service := NewService(store, time.Hour)
	if err := service.EnsureAdmin(context.Background(), "admin", "secret123"); err != nil {
		t.Fatalf("ensure admin: %v", err)
	}

	token, err := service.Login(context.Background(), "admin", "secret123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	session, err := service.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if session.UserID == 0 {
		t.Fatal("expected valid session user id")
	}

	if err := service.Logout(context.Background(), token); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := service.Authenticate(context.Background(), token); err == nil {
		t.Fatal("expected token to be invalid after logout")
	}
}
