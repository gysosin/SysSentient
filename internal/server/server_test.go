package server

import (
	"net/http"
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
