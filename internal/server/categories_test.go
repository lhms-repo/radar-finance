package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// send issues a same origin JSON request with any method.
func send(t *testing.T, c *http.Client, srv *httptest.Server, method, path, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", srv.URL)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// TestCategoryRoutes covers the category API and the mapping of business errors (RF-50, RN-18).
func TestCategoryRoutes(t *testing.T) {
	srv, _ := testServer(t)
	c := newClient(t)
	postJSON(t, c, srv, "/api/auth/login", `{"login":"ana"}`)

	resp := get(t, c, srv.URL+"/api/categories")
	var tree []struct {
		ID       int64 `json:"id"`
		Name     string
		Icon     string
		Children []struct {
			ID       int64 `json:"id"`
			ParentID int64 `json:"parentId"`
			Name     string
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil || len(tree) != 13 {
		t.Fatalf("list = %d categories, %v", len(tree), err)
	}
	moradia := tree[0]
	if moradia.Name != "Moradia" || moradia.Icon != "house" || moradia.Children[0].ParentID != moradia.ID {
		t.Fatalf("first = %+v", moradia)
	}

	resp = postJSON(t, c, srv, "/api/categories", `{"name":"Gás","parentId":1}`)
	var created struct {
		ID   int64 `json:"id"`
		Name string
	}
	_ = json.NewDecoder(resp.Body).Decode(&created)
	if resp.StatusCode != http.StatusCreated || created.Name != "Gás" {
		t.Fatalf("create = %d %+v", resp.StatusCode, created)
	}

	tests := []struct {
		name, method, path, body string
		wantStatus               int
		wantCode                 string
	}{
		{name: "duplicate", method: http.MethodPost, path: "/api/categories", body: `{"name":"gás","parentId":1}`, wantStatus: http.StatusConflict, wantCode: "category_duplicate_name"},
		{name: "missing icon", method: http.MethodPost, path: "/api/categories", body: `{"name":"Igreja"}`, wantStatus: http.StatusBadRequest, wantCode: "category_icon_required"},
		{name: "malformed body", method: http.MethodPost, path: "/api/categories", body: `{`, wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "rename", method: http.MethodPatch, path: "/api/categories/1", body: `{"name":"Casa"}`, wantStatus: http.StatusOK},
		{name: "rename missing", method: http.MethodPatch, path: "/api/categories/9999", body: `{"name":"x"}`, wantStatus: http.StatusNotFound, wantCode: "category_not_found"},
		{name: "move", method: http.MethodPost, path: "/api/categories/2/move", body: `{"parentId":1}`, wantStatus: http.StatusConflict, wantCode: "category_has_children"},
		{name: "delete system category", method: http.MethodDelete, path: "/api/categories/1", wantStatus: http.StatusConflict, wantCode: "category_system"},
		{name: "delete with children", method: http.MethodDelete, path: "/api/categories/2", wantStatus: http.StatusConflict, wantCode: "category_has_children"},
		{name: "delete unused", method: http.MethodDelete, path: "/api/categories/" + strconv.FormatInt(created.ID, 10), wantStatus: http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := send(t, c, srv, tt.method, tt.path, tt.body)
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if tt.wantCode == "" {
				return
			}
			var e struct{ Code, Message string }
			_ = json.NewDecoder(resp.Body).Decode(&e)
			if e.Code != tt.wantCode || e.Message == "" {
				t.Fatalf("error = %+v, want code %s", e, tt.wantCode)
			}
		})
	}
}
