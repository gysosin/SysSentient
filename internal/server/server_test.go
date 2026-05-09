package server

import (
	"net/http"
	"sys-sentient/internal/config"
	"testing"
)

func TestNewHTTPServerSetsProductionTimeouts(t *testing.T) {
	srv := newHTTPServer(":8080", http.NewServeMux())

	if srv.ReadHeaderTimeout <= 0 {
		t.Fatal("ReadHeaderTimeout must be set to defend against slow headers")
	}
	if srv.ReadTimeout <= 0 {
		t.Fatal("ReadTimeout must be set to bound request reads")
	}
	if srv.WriteTimeout <= 0 {
		t.Fatal("WriteTimeout must be set to bound stuck responses")
	}
	if srv.IdleTimeout <= 0 {
		t.Fatal("IdleTimeout must be set to bound idle keep-alive connections")
	}
}

func TestServerOriginPolicyAllowsConfiguredOrigins(t *testing.T) {
	srv := NewServer(config.ServerConfig{
		AllowedOrigins: []string{"http://localhost:8080", "http://localhost:3000"},
	}, nil, nil)

	if !srv.isOriginAllowed("http://localhost:3000") {
		t.Fatal("expected configured origin to be allowed")
	}
	if srv.isOriginAllowed("https://evil.example") {
		t.Fatal("expected unconfigured origin to be rejected")
	}
}

func TestServerOriginPolicyAllowsEmptyOrigin(t *testing.T) {
	srv := NewServer(config.ServerConfig{
		AllowedOrigins: []string{"http://localhost:8080"},
	}, nil, nil)

	if !srv.isOriginAllowed("") {
		t.Fatal("expected empty origin to be allowed for same-origin and non-browser clients")
	}
}
