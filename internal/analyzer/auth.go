package analyzer

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/packages"
)

type AuthAnalyzer struct {
	Acfg       *AnalysisConfig
	ChiPkgName string
	VerboseLog func(format string, args ...interface{})
}

func NewAuthAnalyzer(acfg *AnalysisConfig, chiPkgName string, verboseLog func(string, ...interface{})) *AuthAnalyzer {
	return &AuthAnalyzer{
		Acfg:       acfg,
		ChiPkgName: chiPkgName,
		VerboseLog: verboseLog,
	}
}

type AuthResult struct {
	IsProtected   bool
	SecurityName  string
	MiddlewareName string
	IsLogin       bool
	IsRegister    bool
	IsLogout      bool
}

func (aa *AuthAnalyzer) AnalyzeAuth(route *RouteInfo) *AuthResult {
	result := &AuthResult{
		SecurityName: aa.Acfg.SecurityName,
	}

	if result.SecurityName == "" {
		result.SecurityName = "BearerAuth"
	}

	if route.IsProtected {
		result.IsProtected = true
		result.MiddlewareName = strings.Join(route.Middleware, ", ")
	}

	// Check handler body for auth patterns
	if route.Handler != nil && route.Handler.FuncDecl != nil && route.Handler.FuncDecl.Body != nil {
		if DetectAuthByHeaderRead(route.Handler.FuncDecl.Body, aa.Acfg) {
			result.IsProtected = true
		}
	}

	// Check for login/register/logout
	if route.Handler != nil {
		authType, isAuth := IsAuthRoute(route.Handler.Name, route.Path)
		if isAuth {
			switch authType {
			case "register":
				result.IsRegister = true
			case "login":
				result.IsLogin = true
			case "logout":
				result.IsLogout = true
			}
		}
	}

	return result
}

func (aa *AuthAnalyzer) CheckMiddlewareAuth(file *ast.File, pkg *packages.Package) map[string]*AuthResult {
	results := make(map[string]*AuthResult)

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		if sel.Sel.Name == "Use" {
			for _, arg := range call.Args {
				name := exprToName(arg)
				if isAuthMiddlewareName(name) {
					results[name] = &AuthResult{
						IsProtected:  true,
						SecurityName: aa.Acfg.SecurityName,
					}
				}
			}
		}

		return true
	})

	return results
}

func exprToName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprToName(e.X) + "." + e.Sel.Name
	default:
		return ""
	}
}

func isAuthMiddlewareName(name string) bool {
	lower := strings.ToLower(name)
	patterns := []string{
		"auth", "jwt", "token", "authenticate", "requireauth",
		"bearer", "session", "guard",
	}
	for _, p := range patterns {
		if containsString(lower, p) {
			return true
		}
	}
	return false
}

func DetectAuthInBody(body *ast.BlockStmt) bool {
	if body == nil {
		return false
	}
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		// Check for r.Header.Get("Authorization")
		if sel.Sel.Name == "Get" {
			if callSel, ok2 := sel.X.(*ast.CallExpr); ok2 {
				if headerSel, ok3 := callSel.Fun.(*ast.SelectorExpr); ok3 && headerSel.Sel.Name == "Header" {
					if len(call.Args) > 0 {
						if lit, ok4 := call.Args[0].(*ast.BasicLit); ok4 {
							if strings.Contains(strings.ToLower(lit.Value), "authorization") {
								found = true
								return false
							}
						}
					}
				}
			}
		}
		return true
	})
	return found
}
