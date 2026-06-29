package socialhub

import (
	"net/http"
	"testing"
)

type mockProvider struct{}

func (m *mockProvider) Redirect() string {
	return "mock_redirect"
}

func (m *mockProvider) UserFromCallback(r *http.Request) (SocialUser, error) {
	return SocialUser{ID: "123", Name: "Mock User"}, nil
}

func TestHub_RegistrationAndRetrieval(t *testing.T) {
	hub := New()
	mock := &mockProvider{}

	hub.Register("mock", mock)

	driver, err := hub.Driver("mock")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if driver.Redirect() != "mock_redirect" {
		t.Errorf("Expected mock_redirect, got %s", driver.Redirect())
	}

	_, err = hub.Driver("nonexistent")
	if err == nil {
		t.Errorf("Expected error for nonexistent driver")
	}
}
