package analyzer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func parseSource(t *testing.T, src string) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", "package main\n"+src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return fset, file
}

func TestJoinPaths(t *testing.T) {
	tests := []struct {
		prefix, suffix, expected string
	}{
		{"/api/v1", "/users", "/api/v1/users"},
		{"/api/v1/", "/users", "/api/v1/users"},
		{"/api/v1", "users", "/api/v1/users"},
		{"/users/", "{id}", "/users/{id}"},
		{"/users", "/{id}", "/users/{id}"},
		{"", "/users", "/users"},
		{"/api/v1", "", "/api/v1"},
		{"/", "/ping", "/ping"},
	}
	for _, tc := range tests {
		result := joinPaths(tc.prefix, tc.suffix)
		if result != tc.expected {
			t.Errorf("joinPaths(%q, %q) = %q, want %q", tc.prefix, tc.suffix, result, tc.expected)
		}
	}
}

func TestExtractPathParams(t *testing.T) {
	result := ExtractPathParams("/users/{id}/posts/{postId}")
	if len(result) != 2 || result[0] != "id" || result[1] != "postId" {
		t.Errorf("ExtractPathParams = %v, want [id postId]", result)
	}
}

func TestExtractFirstPathSegment(t *testing.T) {
	if s := ExtractFirstPathSegment("/users/123"); s != "users" {
		t.Errorf("ExtractFirstPathSegment = %q, want users", s)
	}
	if s := ExtractFirstPathSegment("/api/v1/users"); s != "api" {
		t.Errorf("ExtractFirstPathSegment = %q, want api", s)
	}
}

func TestDefaultStatusCode(t *testing.T) {
	tests := map[string]int{
		"GET": 200, "POST": 201, "PUT": 200, "PATCH": 200, "DELETE": 204,
	}
	for method, expected := range tests {
		if got := DefaultStatusCode(method); got != expected {
			t.Errorf("DefaultStatusCode(%s) = %d, want %d", method, got, expected)
		}
	}
}

func TestBuildSummaryFromHandler(t *testing.T) {
	tests := map[string]string{
		"CreateUser":        "Create user",
		"GetUser":           "Get user",
		"ListUsers":         "List users",
		"UpdateUser":        "Update user",
		"DeleteUser":        "Delete user",
		"h.Register":        "Register",
		"h.Login":           "Login",
		"Handler.CreateUser": "Create user",
	}
	for name, expected := range tests {
		got := BuildSummaryFromHandler(name)
		if got != expected {
			t.Errorf("BuildSummaryFromHandler(%q) = %q, want %q", name, got, expected)
		}
	}
}

func TestBuildSummaryFromMethodAndPath(t *testing.T) {
	tests := []struct {
		method, path, expected string
	}{
		{"GET", "/users", "list users"},
		{"GET", "/users/{id}", "get user"},
		{"POST", "/users", "create users"},
		{"PUT", "/users/{id}", "update user"},
		{"DELETE", "/users/{id}", "delete user"},
	}
	for _, tc := range tests {
		got := BuildSummaryFromMethodAndPath(tc.method, tc.path)
		if got != tc.expected {
			t.Errorf("BuildSummaryFromMethodAndPath(%q, %q) = %q, want %q",
				tc.method, tc.path, got, tc.expected)
		}
	}
}

func TestIsAuthRoute(t *testing.T) {
	tests := []struct {
		name, path string
		authType   string
		isAuth     bool
	}{
		{"Register", "/auth/register", "register", true},
		{"h.Register", "/auth/register", "register", true},
		{"SignUp", "/signup", "register", true},
		{"Login", "/auth/login", "login", true},
		{"h.Login", "/auth/signin", "login", true},
		{"Logout", "/auth/logout", "logout", true},
		{"CreateUser", "/api/v1/users", "", false},
		{"ListUsers", "/api/v1/users", "", false},
		{"GetUser", "/users/{id}", "", false},
		{"UpdateUser", "/users/{id}", "", false},
	}
	for _, tc := range tests {
		authType, isAuth := IsAuthRoute(tc.name, tc.path)
		if isAuth != tc.isAuth || authType != tc.authType {
			t.Errorf("IsAuthRoute(%q, %q) = (%q, %v), want (%q, %v)",
				tc.name, tc.path, authType, isAuth, tc.authType, tc.isAuth)
		}
	}
}

func TestPluralizeSingularize(t *testing.T) {
	tests := map[string]string{
		"user": "users", "category": "categories", "users": "users",
	}
	for s, expected := range tests {
		if got := Pluralize(s); got != expected {
			t.Errorf("Pluralize(%q) = %q, want %q", s, got, expected)
		}
	}

	singTests := map[string]string{
		"users": "user", "categories": "category",
	}
	for s, expected := range singTests {
		if got := Singularize(s); got != expected {
			t.Errorf("Singularize(%q) = %q, want %q", s, got, expected)
		}
	}
}

func TestCameraCaseToWords(t *testing.T) {
	tests := map[string]string{
		"CreateUser": "Create User",
		"GetUser":    "Get User",
		"ListUsers":  "List Users",
		"HTTPHandler": "H T T P Handler",
	}
	for s, expected := range tests {
		if got := CameraCaseToWords(s); got != expected {
			t.Errorf("CameraCaseToWords(%q) = %q, want %q", s, got, expected)
		}
	}
}

func TestStripBasePath(t *testing.T) {
	tests := []struct {
		path, base, expected string
	}{
		{"/api/v1/users", "/api/v1", "/users"},
		{"/api/v1/users", "", "/api/v1/users"},
		{"/api/v1", "/api/v1", "/"},
	}
	for _, tc := range tests {
		if got := StripBasePath(tc.path, tc.base); got != tc.expected {
			t.Errorf("StripBasePath(%q, %q) = %q, want %q", tc.path, tc.base, got, tc.expected)
		}
	}
}

func TestExtractTag(t *testing.T) {
	if tag := ExtractTag("/api/v1/users", "/api/v1"); tag != "users" {
		t.Errorf("ExtractTag = %q, want users", tag)
	}
	if tag := ExtractTag("/auth/login", ""); tag != "auth" {
		t.Errorf("ExtractTag = %q, want auth", tag)
	}
}

func TestGetFailureCodes(t *testing.T) {
	codes := GetFailureCodes("POST")
	if len(codes) != 3 || codes[0] != 400 || codes[1] != 409 || codes[2] != 500 {
		t.Errorf("GetFailureCodes(POST) = %v, want [400 409 500]", codes)
	}

	codes = GetFailureCodes("DELETE")
	if len(codes) != 3 || codes[0] != 400 || codes[1] != 404 || codes[2] != 500 {
		t.Errorf("GetFailureCodes(DELETE) = %v, want [400 404 500]", codes)
	}
}

func TestBodyAnalyzerRequest(t *testing.T) {
	src := `
import (
	"encoding/json"
	"net/http"
)
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	resp := UserResponse{}
	json.NewEncoder(w).Encode(resp)
}
`
	fset, file := parseSource(t, src)
	var funcDecl *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			funcDecl = fn
			break
		}
	}
	if funcDecl == nil {
		t.Fatal("no func decl found")
	}

	handler := &HandlerInfo{
		Name:     "h.CreateUser",
		FuncDecl: funcDecl,
		File:     file,
		Fset:     fset,
	}

	acfg := &AnalysisConfig{
		InferRequestBody:  true,
		InferResponseBody: true,
		InferStatusCodes:  true,
	}
	ba := NewBodyAnalyzer(acfg, "chi", nil)
	result := ba.AnalyzeHandler(handler)

	if result.RequestBody == nil {
		t.Fatal("expected request body to be found")
	}
	if result.RequestBody.TypeName != "CreateUserRequest" {
		t.Errorf("request body type = %q, want CreateUserRequest", result.RequestBody.TypeName)
	}
	if result.ResponseBody == nil {
		t.Fatal("expected response body to be found")
	}
	if result.ResponseBody.TypeName != "UserResponse" {
		t.Errorf("response body type = %q, want UserResponse", result.ResponseBody.TypeName)
	}
}

func TestBodyAnalyzerResponseArray(t *testing.T) {
	src := `
import (
	"encoding/json"
	"net/http"
)
func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	json.NewEncoder(w).Encode(v)
}
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users := []UserResponse{{}, {}}
	writeJSON(w, http.StatusOK, users)
}
`
	fset, file := parseSource(t, src)
	var funcDecl *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "ListUsers" {
			funcDecl = fn
			break
		}
	}
	if funcDecl == nil {
		t.Fatal("no func decl found")
	}

	handler := &HandlerInfo{
		Name:     "h.ListUsers",
		FuncDecl: funcDecl,
		File:     file,
		Fset:     fset,
	}

	acfg := &AnalysisConfig{
		InferResponseBody: true,
	}
	ba := NewBodyAnalyzer(acfg, "chi", nil)
	result := ba.AnalyzeHandler(handler)

	if result.ResponseBody == nil {
		t.Fatal("expected response body to be found")
	}
	if !result.ResponseBody.IsArray {
		t.Error("expected response to be array")
	}
	if result.ResponseBody.TypeName != "[]UserResponse" {
		t.Errorf("response type = %q, want []UserResponse", result.ResponseBody.TypeName)
	}
}

func TestBodyAnalyzerQueryParams(t *testing.T) {
	src := `
import "net/http"
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	page := r.URL.Query().Get("page")
	limit := r.URL.Query().Get("limit")
	_ = page
	_ = limit
	w.WriteHeader(http.StatusOK)
}
`
	fset, file := parseSource(t, src)
	var funcDecl *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			funcDecl = fn
			break
		}
	}

	handler := &HandlerInfo{Name: "h.ListUsers", FuncDecl: funcDecl, File: file, Fset: fset}
	acfg := &AnalysisConfig{InferQueryParams: true}
	ba := NewBodyAnalyzer(acfg, "chi", nil)
	result := ba.AnalyzeHandler(handler)

	if len(result.QueryParams) != 2 {
		t.Fatalf("expected 2 query params, got %d", len(result.QueryParams))
	}
	names := map[string]bool{}
	for _, qp := range result.QueryParams {
		names[qp.Name] = true
	}
	if !names["page"] || !names["limit"] {
		t.Errorf("expected query params page and limit, got %v", result.QueryParams)
	}
}

func TestBodyAnalyzerHTTPError(t *testing.T) {
	src := `
import "net/http"
func (h *Handler) BadHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "bad request", http.StatusBadRequest)
}
`
	fset, file := parseSource(t, src)
	var funcDecl *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			funcDecl = fn
			break
		}
	}

	handler := &HandlerInfo{Name: "h.BadHandler", FuncDecl: funcDecl, File: file, Fset: fset}
	acfg := &AnalysisConfig{DefaultErrorType: "ErrorResponse"}
	ba := NewBodyAnalyzer(acfg, "chi", nil)
	result := ba.AnalyzeHandler(handler)

	if len(result.ErrorResponses) != 1 {
		t.Fatalf("expected 1 error response, got %d", len(result.ErrorResponses))
	}
	if result.ErrorResponses[0].StatusCode != 400 {
		t.Errorf("error status = %d, want 400", result.ErrorResponses[0].StatusCode)
	}
}

func TestBodyAnalyzerPathParam(t *testing.T) {
	src := `
import (
	"net/http"
	"strconv"
	"github.com/go-chi/chi/v5"
)
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	_ = id
	w.WriteHeader(http.StatusOK)
}
`
	fset, file := parseSource(t, src)
	var funcDecl *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			funcDecl = fn
			break
		}
	}

	handler := &HandlerInfo{Name: "h.GetUser", FuncDecl: funcDecl, File: file, Fset: fset}
	acfg := &AnalysisConfig{InferPathParams: true}
	ba := NewBodyAnalyzer(acfg, "chi", nil)
	result := ba.AnalyzeHandler(handler)

	if len(result.PathParams) != 1 {
		t.Fatalf("expected 1 path param, got %d", len(result.PathParams))
	}
	if result.PathParams[0].Name != "id" || result.PathParams[0].Type != "int" {
		t.Errorf("path param = %+v, want id(int)", result.PathParams[0])
	}
}

func TestBodyAnalyzerNoContent(t *testing.T) {
	src := `
import "net/http"
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
`
	fset, file := parseSource(t, src)
	var funcDecl *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			funcDecl = fn
			break
		}
	}

	handler := &HandlerInfo{Name: "h.DeleteUser", FuncDecl: funcDecl, File: file, Fset: fset}
	acfg := &AnalysisConfig{InferResponseBody: true, InferStatusCodes: true}
	ba := NewBodyAnalyzer(acfg, "chi", nil)
	result := ba.AnalyzeHandler(handler)

	if result.ResponseBody != nil {
		t.Error("expected no response body for 204 No Content")
	}
	if _, ok := result.AllStatusCodes[204]; !ok {
		t.Error("expected 204 status code to be found")
	}
}

func TestAuthAnalyzer(t *testing.T) {
	acfg := &AnalysisConfig{SecurityName: "BearerAuth"}
	aa := NewAuthAnalyzer(acfg, "chi", nil)

	route := &RouteInfo{
		Method:      "GET",
		Path:        "/api/v1/users",
		IsProtected: true,
		Middleware:   []string{"AuthMiddleware"},
		Handler:     &HandlerInfo{Name: "h.ListUsers"},
	}

	result := aa.AnalyzeAuth(route)

	if !result.IsProtected {
		t.Error("expected route to be protected")
	}
	if result.SecurityName != "BearerAuth" {
		t.Errorf("security name = %q, want BearerAuth", result.SecurityName)
	}
}

func TestAuthAnalyzerLogin(t *testing.T) {
	acfg := &AnalysisConfig{SecurityName: "BearerAuth"}
	aa := NewAuthAnalyzer(acfg, "chi", nil)

	route := &RouteInfo{
		Method:  "POST",
		Path:    "/auth/login",
		Handler: &HandlerInfo{Name: "h.Login"},
	}

	result := aa.AnalyzeAuth(route)

	if !result.IsLogin {
		t.Error("expected route to be detected as login")
	}
}

func TestAuthAnalyzerRegister(t *testing.T) {
	acfg := &AnalysisConfig{SecurityName: "BearerAuth"}
	aa := NewAuthAnalyzer(acfg, "chi", nil)

	route := &RouteInfo{
		Method:  "POST",
		Path:    "/auth/register",
		Handler: &HandlerInfo{Name: "h.SignUp"},
	}

	result := aa.AnalyzeAuth(route)

	if !result.IsRegister {
		t.Error("expected route to be detected as register")
	}
}

func TestIsAuthMiddlewareName(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"AuthMiddleware", true},
		{"JWTGuard", true},
		{"RequireAuth", true},
		{"LoggingMiddleware", false},
		{"CORSMiddleware", false},
	}
	for _, tc := range tests {
		if got := isAuthMiddlewareName(tc.name); got != tc.expected {
			t.Errorf("isAuthMiddlewareName(%q) = %v, want %v", tc.name, got, tc.expected)
		}
	}
}

func TestStatusCodes(t *testing.T) {
	if code := DefaultStatusCode("POST"); code != 201 {
		t.Errorf("default POST status = %d, want 201", code)
	}
	if code := DefaultStatusCode("DELETE"); code != 204 {
		t.Errorf("default DELETE status = %d, want 204", code)
	}
}
