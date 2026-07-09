package rewriter

import (
	"go/ast"
	"os"
	"strings"
	"testing"
)

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
		{[]string{"// @Failure 400"}, true},
		{[]string{"// @Param x"}, true},
		{[]string{"// @Description x"}, true},
		{[]string{"// @Tags x"}, true},
		{[]string{"// @Accept json"}, true},
		{[]string{"// @Produce json"}, true},
		{[]string{"// @Security Bearer"}, true},
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
		if got := hasExistingSwagger(doc); got != tc.expected {
			t.Errorf("hasExistingSwagger(%v) = %v, want %v", tc.comments, got, tc.expected)
		}
	}
}

func TestRewriterDryRun(t *testing.T) {
	rw := New(true, false, nil)

	lineComments := map[int]string{
		10: "// Test godoc\n// @Summary Test",
	}

	// Create a temp file
	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/test.go"
	content := "package main\n\nfunc main() {\n}\n"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	err := rw.applyToFile(tmpFile, lineComments)
	if err != nil {
		t.Fatal(err)
	}

	// File should NOT be modified in dry run
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "@Summary") {
		t.Error("file should not be modified in dry-run mode")
	}
}

func TestRewriterWrite(t *testing.T) {
	rw := New(false, false, nil)

	lineComments := map[int]string{
		3: "// Test godoc\n// @Summary Test\n// @Router /test [get]",
	}

	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/test.go"
	content := "package main\n\n\nfunc main() {\n}\n"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	err := rw.applyToFile(tmpFile, lineComments)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "@Summary Test") {
		t.Error("file should contain @Summary Test")
	}
	if !strings.Contains(string(data), "func main()") {
		t.Error("function should still exist")
	}
}

func TestRewriterMultipleInserts(t *testing.T) {
	rw := New(false, false, nil)

	lineComments := map[int]string{
		4: "// One godoc\n// @Summary One",
		7: "// Two godoc\n// @Summary Two",
	}

	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/test.go"
	content := "package main\n\n\nfunc one() {}\n\n\nfunc two() {}\n"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	err := rw.applyToFile(tmpFile, lineComments)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "@Summary One") {
		t.Error("file should contain @Summary One")
	}
	if !strings.Contains(string(data), "@Summary Two") {
		t.Error("file should contain @Summary Two")
	}
}
