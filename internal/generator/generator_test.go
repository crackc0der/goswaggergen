package generator

import (
	"go/ast"
	"strings"
	"testing"

	"goswaggergen/internal/analyzer"
)

func TestGenerateComment(t *testing.T) {
	acfg := &analyzer.AnalysisConfig{
		BasePath:         "/api/v1",
		DefaultErrorType: "ErrorResponse",
		SecurityName:     "BearerAuth",
	}
	sg := NewSwaggerGen(acfg)

	route := &analyzer.RouteInfo{
		Method:      "POST",
		Path:        "/auth/register",
		IsProtected: false,
		Handler:     &analyzer.HandlerInfo{Name: "h.Register"},
	}

	bodyAnalysis := &analyzer.HandlerAnalysisResult{
		RequestBody: &analyzer.RequestBodyInfo{TypeName: "RegisterRequest", Required: true},
		ResponseBody: &analyzer.ResponseBodyInfo{TypeName: "AuthResponse", StatusCode: 201},
		ErrorResponses: []analyzer.ErrorResponseInfo{{StatusCode: 400, TypeName: "ErrorResponse"}},
		AllStatusCodes: map[int]bool{201: true, 400: true},
	}

	authResult := &analyzer.AuthResult{
		IsRegister:   true,
		SecurityName: "BearerAuth",
	}

	comment := sg.GenerateComment(route, bodyAnalysis, authResult)

	if !strings.Contains(comment, "@Summary Register user") {
		t.Error("comment should contain @Summary Register user")
	}
	if !strings.Contains(comment, "@Tags auth") {
		t.Error("comment should contain @Tags auth")
	}
	if !strings.Contains(comment, "@Router /auth/register [post]") {
		t.Error("comment should contain @Router /auth/register [post]")
	}
	if !strings.Contains(comment, "@Param request body RegisterRequest true") {
		t.Error("comment should contain request body param")
	}
	if !strings.Contains(comment, "@Success 201 {object} AuthResponse") {
		t.Error("comment should contain success response")
	}
	if !strings.Contains(comment, "@Failure 400 {object} ErrorResponse") {
		t.Error("comment should contain failure response")
	}
}

func TestGenerateCommentProtected(t *testing.T) {
	acfg := &analyzer.AnalysisConfig{DefaultErrorType: "ErrorResponse", SecurityName: "BearerAuth"}
	sg := NewSwaggerGen(acfg)

	route := &analyzer.RouteInfo{
		Method:      "GET",
		Path:        "/api/v1/users/{id}",
		IsProtected: true,
		AuthName:    "BearerAuth",
		Handler:     &analyzer.HandlerInfo{Name: "h.GetUser"},
	}

	bodyAnalysis := &analyzer.HandlerAnalysisResult{
		ResponseBody:   &analyzer.ResponseBodyInfo{TypeName: "UserResponse", StatusCode: 200},
		ErrorResponses: []analyzer.ErrorResponseInfo{{StatusCode: 404, TypeName: "ErrorResponse"}},
		PathParams:     []analyzer.ParamInfo{{Name: "id", In: "path", Type: "int", Required: true}},
	}

	authResult := &analyzer.AuthResult{IsProtected: true, SecurityName: "BearerAuth"}

	comment := sg.GenerateComment(route, bodyAnalysis, authResult)

	if !strings.Contains(comment, "@Security BearerAuth") {
		t.Error("protected route should have @Security")
	}
	if !strings.Contains(comment, "@Param id path int true") {
		t.Error("comment should contain path param id")
	}
}

func TestGenerateCommentArrayResponse(t *testing.T) {
	acfg := &analyzer.AnalysisConfig{DefaultErrorType: "ErrorResponse"}
	sg := NewSwaggerGen(acfg)

	route := &analyzer.RouteInfo{
		Method:  "GET",
		Path:    "/api/v1/users",
		Handler: &analyzer.HandlerInfo{Name: "h.ListUsers"},
	}

	bodyAnalysis := &analyzer.HandlerAnalysisResult{
		ResponseBody: &analyzer.ResponseBodyInfo{TypeName: "[]UserResponse", StatusCode: 200, IsArray: true},
	}

	authResult := &analyzer.AuthResult{}

	comment := sg.GenerateComment(route, bodyAnalysis, authResult)

	if !strings.Contains(comment, "@Success 200 {array} UserResponse") {
		t.Errorf("expected {array} UserResponse in: %s", comment)
	}
	if strings.Contains(comment, "[]UserResponse") {
		t.Error("comment should not contain [] prefix in response type")
	}
}

func TestGenerateCommentNoContent(t *testing.T) {
	acfg := &analyzer.AnalysisConfig{DefaultErrorType: "ErrorResponse"}
	sg := NewSwaggerGen(acfg)

	route := &analyzer.RouteInfo{
		Method:  "DELETE",
		Path:    "/api/v1/users/{id}",
		Handler: &analyzer.HandlerInfo{Name: "h.DeleteUser"},
	}

	bodyAnalysis := &analyzer.HandlerAnalysisResult{
		AllStatusCodes: map[int]bool{204: true},
	}

	authResult := &analyzer.AuthResult{}

	comment := sg.GenerateComment(route, bodyAnalysis, authResult)

	if !strings.Contains(comment, "@Success 204") {
		t.Error("should have @Success 204")
	}
	// Success line for 204 should not have object type
	lines := strings.Split(comment, "\n")
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "// @Success 204") {
			if strings.Contains(line, "{object}") {
				t.Error("Success 204 should not contain object type")
			}
		}
	}
}

func TestGenerateCommentLogin(t *testing.T) {
	acfg := &analyzer.AnalysisConfig{DefaultErrorType: "ErrorResponse"}
	sg := NewSwaggerGen(acfg)

	route := &analyzer.RouteInfo{
		Method:  "POST",
		Path:    "/auth/login",
		Handler: &analyzer.HandlerInfo{Name: "h.Login"},
	}

	bodyAnalysis := &analyzer.HandlerAnalysisResult{
		RequestBody: &analyzer.RequestBodyInfo{TypeName: "LoginRequest", Required: true},
		ResponseBody: &analyzer.ResponseBodyInfo{TypeName: "AuthResponse", StatusCode: 200},
	}

	authResult := &analyzer.AuthResult{IsLogin: true}

	comment := sg.GenerateComment(route, bodyAnalysis, authResult)

	if !strings.Contains(comment, "@Summary Login user") {
		t.Error("should have @Summary Login user")
	}
	if !strings.Contains(comment, "@Tags auth") {
		t.Error("should have @Tags auth")
	}
}

func TestHasExistingSwagger(t *testing.T) {
	tests := []struct {
		comments []string
		expected bool
	}{
		{nil, false},
		{[]string{}, false},
		{[]string{"// regular comment"}, false},
		{[]string{"// @Summary test"}, true},
		{[]string{"// @Router /test [get]"}, true},
		{[]string{"// @Success 200"}, true},
	}

	for _, tc := range tests {
		var doc *ast.CommentGroup
		if tc.comments != nil {
			list := make([]*ast.Comment, len(tc.comments))
			for i, c := range tc.comments {
				list[i] = &ast.Comment{Text: c}
			}
			doc = &ast.CommentGroup{List: list}
		}
		if got := HasExistingSwagger(doc); got != tc.expected {
			t.Errorf("HasExistingSwagger(%v) = %v, want %v", tc.comments, got, tc.expected)
		}
	}
}
