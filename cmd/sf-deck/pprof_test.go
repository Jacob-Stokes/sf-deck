package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPprofHandlerRequiresToken(t *testing.T) {
	handler := newPprofHandler("test-token")

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	authorized := httptest.NewRecorder()
	authorizedRequest := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	authorizedRequest.SetBasicAuth("sfdeck", "test-token")
	handler.ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized status = %d", authorized.Code)
	}

	withBearer := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine?debug=1", nil)
	withBearer.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, withBearer)
	if response.Code != http.StatusOK {
		t.Fatalf("bearer status = %d", response.Code)
	}
}
