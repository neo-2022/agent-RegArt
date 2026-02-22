package auth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseTokens(t *testing.T) {
	tokens := ParseTokens("tok1:viewer,tok2:operator,tok3:admin")
	if len(tokens) != 3 {
		t.Fatalf("ожидалось 3 токена, получено %d", len(tokens))
	}
	if tokens["tok1"] != RoleViewer {
		t.Errorf("tok1: ожидалась viewer, получена %s", tokens["tok1"])
	}
	if tokens["tok2"] != RoleOperator {
		t.Errorf("tok2: ожидалась operator, получена %s", tokens["tok2"])
	}
	if tokens["tok3"] != RoleAdmin {
		t.Errorf("tok3: ожидалась admin, получена %s", tokens["tok3"])
	}
}

func TestParseTokens_Empty(t *testing.T) {
	tokens := ParseTokens("")
	if len(tokens) != 0 {
		t.Fatalf("ожидалось 0 токенов, получено %d", len(tokens))
	}
}

func TestParseTokens_InvalidFormat(t *testing.T) {
	tokens := ParseTokens("badtoken,tok2:viewer")
	if len(tokens) != 1 {
		t.Fatalf("ожидался 1 токен, получено %d", len(tokens))
	}
}

func TestParseTokens_InvalidRole(t *testing.T) {
	tokens := ParseTokens("tok1:superadmin")
	if len(tokens) != 0 {
		t.Fatalf("ожидалось 0 токенов (невалидная роль), получено %d", len(tokens))
	}
}

func TestHasAccess(t *testing.T) {
	tests := []struct {
		actual   Role
		required Role
		want     bool
	}{
		{RoleAdmin, RoleAdmin, true},
		{RoleAdmin, RoleOperator, true},
		{RoleAdmin, RoleViewer, true},
		{RoleOperator, RoleOperator, true},
		{RoleOperator, RoleViewer, true},
		{RoleOperator, RoleAdmin, false},
		{RoleViewer, RoleViewer, true},
		{RoleViewer, RoleOperator, false},
		{RoleViewer, RoleAdmin, false},
	}
	for _, tc := range tests {
		got := HasAccess(tc.actual, tc.required)
		if got != tc.want {
			t.Errorf("HasAccess(%s, %s) = %v, ожидалось %v", tc.actual, tc.required, got, tc.want)
		}
	}
}

func TestWithAuth_LegacyMode(t *testing.T) {
	handler := WithAuth(RoleAdmin, map[string]Role{}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("legacy-режим: ожидался 200, получен %d", rr.Code)
	}
}

func TestWithAuth_NoHeader(t *testing.T) {
	tokens := map[string]Role{"tok1": RoleAdmin}
	handler := WithAuth(RoleAdmin, tokens, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("без заголовка: ожидался 401, получен %d", rr.Code)
	}
}

func TestWithAuth_InvalidToken(t *testing.T) {
	tokens := map[string]Role{"tok1": RoleAdmin}
	handler := WithAuth(RoleAdmin, tokens, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("невалидный токен: ожидался 401, получен %d", rr.Code)
	}
}

func TestWithAuth_InsufficientRole(t *testing.T) {
	tokens := map[string]Role{"tok1": RoleViewer}
	handler := WithAuth(RoleAdmin, tokens, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer tok1")
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("недостаточно прав: ожидался 403, получен %d", rr.Code)
	}
}

func TestWithAuth_ValidToken(t *testing.T) {
	tokens := map[string]Role{"tok1": RoleAdmin}
	handler := WithAuth(RoleAdmin, tokens, func(w http.ResponseWriter, r *http.Request) {
		role := RoleFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, string(role))
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer tok1")
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("валидный токен: ожидался 200, получен %d", rr.Code)
	}
	if rr.Body.String() != "admin" {
		t.Errorf("роль: ожидалась admin, получена %s", rr.Body.String())
	}
}

func TestWithAuth_OperatorAccessesViewer(t *testing.T) {
	tokens := map[string]Role{"tok1": RoleOperator}
	handler := WithAuth(RoleViewer, tokens, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer tok1")
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("operator→viewer: ожидался 200, получен %d", rr.Code)
	}
}

func TestWithAuth_BadFormat(t *testing.T) {
	tokens := map[string]Role{"tok1": RoleAdmin}
	handler := WithAuth(RoleAdmin, tokens, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic tok1")
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("неправильный формат: ожидался 401, получен %d", rr.Code)
	}
}
