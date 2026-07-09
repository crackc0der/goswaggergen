package analyzer

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/packages"
)

var chiMethods = map[string]bool{
	"Get":        true,
	"Post":       true,
	"Put":        true,
	"Patch":      true,
	"Delete":     true,
	"Options":    true,
	"Head":       true,
	"Method":     true,
	"MethodFunc": true,
	"Route":      true,
	"Group":      true,
	"With":       true,
	"Mount":      true,
	"Use":        true,
	"Handle":     true,
	"HandleFunc": true,
}

var httpMethods = map[string]bool{
	"GET":     true,
	"POST":    true,
	"PUT":     true,
	"PATCH":   true,
	"DELETE":  true,
	"OPTIONS": true,
	"HEAD":    true,
}

type RouteFinder struct {
	Acfg       *AnalysisConfig
	ChiPkgName string
	ChiImpPath string
	VerboseLog func(format string, args ...interface{})
}

func NewRouteFinder(acfg *AnalysisConfig, chiPkgName, chiImpPath string, verboseLog func(string, ...interface{})) *RouteFinder {
	return &RouteFinder{
		Acfg:       acfg,
		ChiPkgName: chiPkgName,
		ChiImpPath: chiImpPath,
		VerboseLog: verboseLog,
	}
}

func (rf *RouteFinder) FindRoutes(pkg *packages.Package, pkgs []*packages.Package) []*RouteInfo {
	var routes []*RouteInfo
	for _, file := range pkg.Syntax {
		routes = append(routes, rf.findRoutesInFile(file, pkg, "", make([]string, 0), false, "")...)
	}
	_ = pkgs
	return routes
}

func (rf *RouteFinder) findRoutesInFile(file *ast.File, pkg *packages.Package, prefix string, middleware []string, protected bool, authName string) []*RouteInfo {
	var routes []*RouteInfo

	ast.Inspect(file, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := callExpr.Fun.(*ast.SelectorExpr)
		if !ok {
			// Check for r.Method(http.MethodGet, path, handler) which is
			// chained from a selector that itself might be the result of
			// a call like r.With(...).Method(...)
			return true
		}

		methodName := sel.Sel.Name
		if !chiMethods[methodName] {
			return true
		}

		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			callInChain, ok := sel.X.(*ast.CallExpr)
			if ok {
				return rf.processCallChain(callInChain, methodName, callExpr, file, pkg, prefix, middleware, protected, authName, &routes)
			}
			return true
		}

		if !rf.isChiRouter(ident, pkg) {
			return true
		}

		rf.processChiCall(ident, methodName, callExpr, file, pkg, prefix, middleware, protected, authName, &routes)
		return true
	})

	return routes
}

func (rf *RouteFinder) processCallChain(chainCall *ast.CallExpr, methodName string, callExpr *ast.CallExpr, file *ast.File, pkg *packages.Package, prefix string, middleware []string, protected bool, authName string, routes *[]*RouteInfo) bool {
	// Handle r.With(middleware).Get(...) and r.Route(...).Get(...)
	chainSel, ok := chainCall.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	chainMethod := chainSel.Sel.Name
	chainX, ok := chainSel.X.(*ast.Ident)
	if !ok {
		return false
	}

	if !rf.isChiRouter(chainX, pkg) {
		return false
	}

	switch chainMethod {
	case "With":
		newMiddleware := rf.extractMiddleware(chainCall.Args)
		allMw := append([]string{}, middleware...)
		allMw = append(allMw, newMiddleware...)
		newProtected := protected || rf.isAuthMiddleware(newMiddleware)
		newAuth := authName
		if newProtected && newAuth == "" {
			newAuth = rf.Acfg.SecurityName
		}
		rf.processChiCall(chainX, methodName, callExpr, file, pkg, prefix, allMw, newProtected, newAuth, routes)
		return false

	case "Route":
		newPrefix := prefix
		if len(chainCall.Args) > 0 {
			if lit, ok := chainCall.Args[0].(*ast.BasicLit); ok {
				pathStr := strings.Trim(lit.Value, `"`)
				newPrefix = joinPaths(newPrefix, pathStr)
			}
		}
		if len(chainCall.Args) > 1 {
			if fnLit, ok := chainCall.Args[1].(*ast.FuncLit); ok {
				subRoutes := rf.findRoutesInFuncLit(fnLit, file, pkg, newPrefix, middleware, protected, authName)
				*routes = append(*routes, subRoutes...)
			}
		}
		return false

	case "Group":
		if len(chainCall.Args) > 0 {
			if fnLit, ok := chainCall.Args[0].(*ast.FuncLit); ok {
				subRoutes := rf.findRoutesInFuncLit(fnLit, file, pkg, prefix, middleware, protected, authName)
				*routes = append(*routes, subRoutes...)
			}
		}
		return false
	}

	return false
}

func (rf *RouteFinder) processChiCall(ident *ast.Ident, methodName string, callExpr *ast.CallExpr, file *ast.File, pkg *packages.Package, prefix string, middleware []string, protected bool, authName string, routes *[]*RouteInfo) {
	switch methodName {
	case "Get", "Post", "Put", "Patch", "Delete", "Options", "Head":
		route := rf.parseRouteMethod(methodName, callExpr, file, pkg, prefix, middleware, protected, authName)
		if route != nil {
			*routes = append(*routes, route)
		}

	case "Method":
		route := rf.parseMethodCall(callExpr, file, pkg, prefix, middleware, protected, authName)
		if route != nil {
			*routes = append(*routes, route)
		}

	case "MethodFunc":
		route := rf.parseMethodFuncCall(callExpr, file, pkg, prefix, middleware, protected, authName)
		if route != nil {
			*routes = append(*routes, route)
		}

	case "Route":
		newPrefix := prefix
		if len(callExpr.Args) > 0 {
			if lit, ok := callExpr.Args[0].(*ast.BasicLit); ok {
				pathStr := strings.Trim(lit.Value, `"`)
				newPrefix = joinPaths(newPrefix, pathStr)
			}
		}
		for i := 1; i < len(callExpr.Args); i++ {
			if fnLit, ok := callExpr.Args[i].(*ast.FuncLit); ok {
				subRoutes := rf.findRoutesInFuncLit(fnLit, file, pkg, newPrefix, middleware, protected, authName)
				*routes = append(*routes, subRoutes...)
			}
		}

	case "Group":
		for _, arg := range callExpr.Args {
			if fnLit, ok := arg.(*ast.FuncLit); ok {
				subRoutes := rf.findRoutesInFuncLit(fnLit, file, pkg, prefix, middleware, protected, authName)
				*routes = append(*routes, subRoutes...)
			}
		}

	case "With":
		newMiddleware := rf.extractMiddleware(callExpr.Args)
		if len(callExpr.Args) > 0 {
			// With returns a router; the next call in chain is processed separately
		}
		_ = newMiddleware

	case "Mount":
		newPrefix := prefix
		if len(callExpr.Args) > 0 {
			if lit, ok := callExpr.Args[0].(*ast.BasicLit); ok {
				pathStr := strings.Trim(lit.Value, `"`)
				newPrefix = joinPaths(newPrefix, pathStr)
			}
		}
		if len(callExpr.Args) > 1 {
			rf.resolveMount(callExpr.Args[1], file, pkg, newPrefix, middleware, protected, authName, routes)
		}

	case "Use":
		if len(callExpr.Args) > 0 {
			// Middleware is applied at this level, handled by parent
		}
	}
}

func (rf *RouteFinder) parseRouteMethod(methodName string, callExpr *ast.CallExpr, file *ast.File, pkg *packages.Package, prefix string, middleware []string, protected bool, authName string) *RouteInfo {
	if len(callExpr.Args) < 2 {
		return nil
	}

	pathStr := ""
	if lit, ok := callExpr.Args[0].(*ast.BasicLit); ok {
		pathStr = strings.Trim(lit.Value, `"`)
	}

	fullPath := joinPaths(prefix, pathStr)
	handlerNode := callExpr.Args[1]
	handlerInfo := rf.resolveHandler(handlerNode, file, pkg)
	if handlerInfo == nil {
		return nil
	}

	route := &RouteInfo{
		Method:      strings.ToUpper(methodName),
		Path:        fullPath,
		Handler:     handlerInfo,
		Middleware:   middleware,
		IsProtected: protected,
		AuthName:    authName,
		RouteNode:   callExpr,
		File:        file,
		Fset:        pkg.Fset,
		Pkg:         pkg,
	}

	return route
}

func (rf *RouteFinder) parseMethodCall(callExpr *ast.CallExpr, file *ast.File, pkg *packages.Package, prefix string, middleware []string, protected bool, authName string) *RouteInfo {
	if len(callExpr.Args) < 3 {
		return nil
	}

	httpMethod := rf.extractHTTPMethod(callExpr.Args[0])
	if httpMethod == "" {
		return nil
	}

	pathStr := ""
	if lit, ok := callExpr.Args[1].(*ast.BasicLit); ok {
		pathStr = strings.Trim(lit.Value, `"`)
	}

	fullPath := joinPaths(prefix, pathStr)
	handlerNode := callExpr.Args[2]
	handlerInfo := rf.resolveHandler(handlerNode, file, pkg)
	if handlerInfo == nil {
		return nil
	}

	return &RouteInfo{
		Method:      httpMethod,
		Path:        fullPath,
		Handler:     handlerInfo,
		Middleware:   middleware,
		IsProtected: protected,
		AuthName:    authName,
		RouteNode:   callExpr,
		File:        file,
		Fset:        pkg.Fset,
		Pkg:         pkg,
	}
}

func (rf *RouteFinder) parseMethodFuncCall(callExpr *ast.CallExpr, file *ast.File, pkg *packages.Package, prefix string, middleware []string, protected bool, authName string) *RouteInfo {
	if len(callExpr.Args) < 3 {
		return nil
	}

	httpMethod := rf.extractHTTPMethod(callExpr.Args[0])
	if httpMethod == "" {
		return nil
	}

	pathStr := ""
	if lit, ok := callExpr.Args[1].(*ast.BasicLit); ok {
		pathStr = strings.Trim(lit.Value, `"`)
	}

	fullPath := joinPaths(prefix, pathStr)
	handlerNode := callExpr.Args[2]
	handlerInfo := rf.resolveHandler(handlerNode, file, pkg)
	if handlerInfo == nil {
		return nil
	}

	return &RouteInfo{
		Method:      httpMethod,
		Path:        fullPath,
		Handler:     handlerInfo,
		Middleware:   middleware,
		IsProtected: protected,
		AuthName:    authName,
		RouteNode:   callExpr,
		File:        file,
		Fset:        pkg.Fset,
		Pkg:         pkg,
	}
}

func (rf *RouteFinder) extractHTTPMethod(expr ast.Expr) string {
	sel, ok := expr.(*ast.SelectorExpr)
	if ok {
		id, ok2 := sel.X.(*ast.Ident)
		if ok2 && id.Name == "http" {
			methodName := sel.Sel.Name
			if strings.HasPrefix(methodName, "Method") {
				m := strings.TrimPrefix(methodName, "Method")
				if httpMethods[strings.ToUpper(m)] {
					return strings.ToUpper(m)
				}
			}
		}
		name := sel.Sel.Name
		if httpMethods[strings.ToUpper(name)] {
			return strings.ToUpper(name)
		}
	}
	return ""
}

func (rf *RouteFinder) findRoutesInFuncLit(fnLit *ast.FuncLit, file *ast.File, pkg *packages.Package, prefix string, middleware []string, protected bool, authName string) []*RouteInfo {
	var routes []*RouteInfo

	ast.Inspect(fnLit, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := callExpr.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		methodName := sel.Sel.Name

		if methodName == "Use" {
			newMw := rf.extractMiddleware(callExpr.Args)
			middleware = append(middleware, newMw...)
			if rf.isAuthMiddleware(newMw) {
				protected = true
				if authName == "" {
					authName = rf.Acfg.SecurityName
				}
			}
			return true
		}

		if methodName == "Group" {
			if len(callExpr.Args) > 0 {
				if innerFn, ok := callExpr.Args[0].(*ast.FuncLit); ok {
					subRoutes := rf.findRoutesInFuncLit(innerFn, file, pkg, prefix, middleware, protected, authName)
					routes = append(routes, subRoutes...)
				}
			}
			return false
		}

		if methodName == "Route" {
			newPrefix := prefix
			if len(callExpr.Args) > 0 {
				if lit, ok := callExpr.Args[0].(*ast.BasicLit); ok {
					pathStr := strings.Trim(lit.Value, `"`)
					newPrefix = joinPaths(newPrefix, pathStr)
				}
			}
			for i := 1; i < len(callExpr.Args); i++ {
				if innerFn, ok := callExpr.Args[i].(*ast.FuncLit); ok {
					subRoutes := rf.findRoutesInFuncLit(innerFn, file, pkg, newPrefix, middleware, protected, authName)
					routes = append(routes, subRoutes...)
				}
			}
			return false
		}

		if methodName == "With" {
			newMw := rf.extractMiddleware(callExpr.Args)
			allMw := append([]string{}, middleware...)
			allMw = append(allMw, newMw...)
			newProtected := protected || rf.isAuthMiddleware(newMw)
			newAuth := authName
			if newProtected && newAuth == "" {
				newAuth = rf.Acfg.SecurityName
			}
			middleware = allMw
			protected = newProtected
			authName = newAuth
			return true
		}

		if methodName == "Mount" {
			newPrefix := prefix
			if len(callExpr.Args) > 0 {
				if lit, ok := callExpr.Args[0].(*ast.BasicLit); ok {
					pathStr := strings.Trim(lit.Value, `"`)
					newPrefix = joinPaths(newPrefix, pathStr)
				}
			}
			if len(callExpr.Args) > 1 {
				rf.resolveMountInScope(callExpr.Args[1], file, pkg, newPrefix, middleware, protected, authName, &routes)
			}
			return false
		}

		if !chiMethods[methodName] {
			return true
		}

		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}

		if !rf.isChiRouterParam(ident, fnLit) {
			return true
		}

		currentMw := make([]string, len(middleware))
		copy(currentMw, middleware)

		rf.processChiCall(ident, methodName, callExpr, file, pkg, prefix, currentMw, protected, authName, &routes)
		return true
	})

	return routes
}

func (rf *RouteFinder) resolveMount(expr ast.Expr, file *ast.File, pkg *packages.Package, prefix string, middleware []string, protected bool, authName string, routes *[]*RouteInfo) {
	call, ok := expr.(*ast.CallExpr)
	if ok {
		rf.VerboseLog("warning: cannot resolve mounted router %s", prefix)
		_ = call
		return
	}
	rf.VerboseLog("warning: cannot resolve mounted router %s", prefix)
}

func (rf *RouteFinder) resolveMountInScope(expr ast.Expr, file *ast.File, pkg *packages.Package, prefix string, middleware []string, protected bool, authName string, routes *[]*RouteInfo) {
	rf.resolveMount(expr, file, pkg, prefix, middleware, protected, authName, routes)
}

func (rf *RouteFinder) isChiRouter(ident *ast.Ident, pkg *packages.Package) bool {
	if ident.Obj != nil {
		if assignStmt, ok := ident.Obj.Decl.(*ast.AssignStmt); ok {
			for _, rhs := range assignStmt.Rhs {
				if call, ok := rhs.(*ast.CallExpr); ok {
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
						xid, ok2 := sel.X.(*ast.Ident)
						if ok2 && xid.Name == rf.ChiPkgName && sel.Sel.Name == "NewRouter" {
							return true
						}
					}
				}
			}
		}
		if valSpec, ok := ident.Obj.Decl.(*ast.ValueSpec); ok {
			for _, val := range valSpec.Values {
				if call, ok := val.(*ast.CallExpr); ok {
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
						xid, ok2 := sel.X.(*ast.Ident)
						if ok2 && xid.Name == rf.ChiPkgName && sel.Sel.Name == "NewRouter" {
							return true
						}
					}
				}
			}
		}
	}

	if ident.Obj == nil {
		return rf.isChiRouterParam(ident, nil)
	}

	return false
}

func (rf *RouteFinder) isChiRouterParam(ident *ast.Ident, fnLit *ast.FuncLit) bool {
	if fnLit != nil && fnLit.Type != nil {
		for _, param := range fnLit.Type.Params.List {
			for _, name := range param.Names {
				if name.Name == ident.Name {
					sel, ok := param.Type.(*ast.SelectorExpr)
					if ok {
						xid, ok2 := sel.X.(*ast.Ident)
						if ok2 && xid.Name == rf.ChiPkgName && sel.Sel.Name == "Router" {
							return true
						}
					}
					return true
				}
			}
		}
	}
	return true
}

func (rf *RouteFinder) extractMiddleware(args []ast.Expr) []string {
	var mw []string
	for _, arg := range args {
		name := rf.exprToString(arg)
		if name != "" {
			mw = append(mw, name)
		}
	}
	return mw
}

func (rf *RouteFinder) exprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return rf.exprToString(e.X) + "." + e.Sel.Name
	case *ast.FuncLit:
		return "func_literal"
	default:
		return ""
	}
}

func (rf *RouteFinder) HandlerName(node ast.Expr) string {
	switch e := node.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return rf.exprToString(e.X) + "." + e.Sel.Name
	case *ast.CallExpr:
		return rf.exprToString(e.Fun)
	default:
		return ""
	}
}

func (rf *RouteFinder) resolveHandler(expr ast.Expr, file *ast.File, pkg *packages.Package) *HandlerInfo {
	switch e := expr.(type) {
	case *ast.Ident:
		name := e.Name
		decl := rf.findFuncDecl(name, file, pkg)
		if decl != nil {
			return &HandlerInfo{
				Name:     name,
				FuncDecl: decl,
				File:     file,
				Fset:     pkg.Fset,
				PkgPath:  pkg.PkgPath,
				Pkg:      pkg,
			}
		}
		return &HandlerInfo{
			Name: name,
			File: file,
			Fset: pkg.Fset,
			PkgPath: pkg.PkgPath,
			Pkg: pkg,
		}

	case *ast.SelectorExpr:
		name := rf.exprToString(e.X) + "." + e.Sel.Name
		// Handle h.CreateUser - find method on receiver type
		decl := rf.findMethodDecl(e, file, pkg)
		if decl != nil {
			return &HandlerInfo{
				Name:     name,
				FuncDecl: decl,
				File:     file,
				Fset:     pkg.Fset,
				PkgPath:  pkg.PkgPath,
				Pkg:      pkg,
				IsMethod: true,
				Receiver: rf.exprToString(e.X),
			}
		}
		// Try to find as package-level function in another package
		pkgName := rf.exprToString(e.X)
		decl = rf.findFuncDeclInPkg(e.Sel.Name, pkgName, file, pkg)
		if decl != nil {
			return &HandlerInfo{
				Name:     name,
				FuncDecl: decl,
				File:     file,
				Fset:     pkg.Fset,
				PkgPath:  pkg.PkgPath,
				Pkg:      pkg,
			}
		}
		return &HandlerInfo{
			Name: name,
			File: file,
			Fset: pkg.Fset,
			PkgPath: pkg.PkgPath,
			Pkg: pkg,
		}

	case *ast.FuncLit:
		return &HandlerInfo{
			Name:     "",
			FuncDecl: &ast.FuncDecl{Type: e.Type, Body: e.Body},
			File:     file,
			Fset:     pkg.Fset,
			PkgPath:  pkg.PkgPath,
			Pkg:      pkg,
			IsAnon:   true,
		}

	case *ast.CallExpr:
		// Wrapped handler like withLogging(h.CreateUser)
		inner := rf.unwrapHandler(e)
		if inner != nil {
			info := rf.resolveHandler(inner, file, pkg)
			if info != nil {
				return info
			}
		}
		name := rf.exprToString(e.Fun)
		return &HandlerInfo{
			Name:     name,
			File:     file,
			Fset:     pkg.Fset,
			PkgPath:  pkg.PkgPath,
			Pkg:      pkg,
			CallExpr: e,
		}

	default:
		return nil
	}
}

func (rf *RouteFinder) unwrapHandler(expr *ast.CallExpr) ast.Expr {
	// Check for http.HandlerFunc(h.CreateUser)
	if sel, ok := expr.Fun.(*ast.SelectorExpr); ok {
		if xid, ok2 := sel.X.(*ast.Ident); ok2 {
			if xid.Name == "http" && sel.Sel.Name == "HandlerFunc" {
				if len(expr.Args) > 0 {
					return expr.Args[0]
				}
			}
		}
	}

	// Check for middleware.Wrap(h.CreateUser) - any single-arg wrapper
	if len(expr.Args) == 1 {
		return expr.Args[0]
	}

	return nil
}

func (rf *RouteFinder) findFuncDecl(name string, file *ast.File, pkg *packages.Package) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Recv == nil && fn.Name.Name == name {
			return fn
		}
	}
	return nil
}

func (rf *RouteFinder) findMethodDecl(sel *ast.SelectorExpr, file *ast.File, pkg *packages.Package) *ast.FuncDecl {
	methodName := sel.Sel.Name
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Recv != nil && fn.Name.Name == methodName {
			return fn
		}
	}
	return nil
}

func (rf *RouteFinder) findFuncDeclInPkg(name, pkgName string, file *ast.File, pkg *packages.Package) *ast.FuncDecl {
	// Look through all files in the package
	for _, f := range pkg.Syntax {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if fn.Recv == nil && fn.Name.Name == name {
				return fn
			}
		}
	}
	return nil
}

func (rf *RouteFinder) isAuthMiddleware(names []string) bool {
	authPatterns := []string{
		"Auth", "auth", "JWT", "jwt", "Token", "token",
		"Authenticate", "RequireAuth", "Bearer", "Session",
	}
	for _, name := range names {
		for _, pattern := range authPatterns {
			if containsString(name, pattern) {
				return true
			}
		}
	}
	return false
}

func joinPaths(prefix, suffix string) string {
	prefix = strings.TrimSuffix(prefix, "/")
	if suffix == "" {
		return prefix
	}
	if !strings.HasPrefix(suffix, "/") {
		suffix = "/" + suffix
	}
	if prefix == "" {
		return suffix
	}
	return prefix + suffix
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func extractStringFromBasicLit(lit *ast.BasicLit) string {
	if lit == nil {
		return ""
	}
	return strings.Trim(lit.Value, `"`)
}

func ExtractPathParams(path string) []string {
	var params []string
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			params = append(params, part[1:len(part)-1])
		}
	}
	return params
}

func ExtractFirstPathSegment(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return ""
}

func StripBasePath(path, basePath string) string {
	if basePath == "" {
		return path
	}
	trimmed := strings.TrimPrefix(path, basePath)
	if trimmed == "" {
		return "/"
	}
	return trimmed
}

func DefaultStatusCode(method string) int {
	switch strings.ToUpper(method) {
	case "GET":
		return 200
	case "POST":
		return 201
	case "PUT":
		return 200
	case "PATCH":
		return 200
	case "DELETE":
		return 204
	default:
		return 200
	}
}

func MethodToOperation(method, path string) string {
	switch strings.ToUpper(method) {
	case "GET":
		if containsPathParam(path) {
			return "Get"
		}
		return "List"
	case "POST":
		return "Create"
	case "PUT":
		return "Update"
	case "PATCH":
		return "Patch"
	case "DELETE":
		return "Delete"
	default:
		return strings.ToUpper(method[:1]) + strings.ToLower(method[1:])
	}
}

func containsPathParam(path string) bool {
	return strings.Contains(path, "{")
}

func Pluralize(s string) string {
	if strings.HasSuffix(s, "s") {
		return s
	}
	if strings.HasSuffix(s, "y") {
		return s[:len(s)-1] + "ies"
	}
	return s + "s"
}

func Singularize(s string) string {
	if strings.HasSuffix(s, "ies") {
		return s[:len(s)-3] + "y"
	}
	if strings.HasSuffix(s, "s") {
		return s[:len(s)-1]
	}
	return s
}

func CameraCaseToWords(s string) string {
	var words []string
	var current []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			words = append(words, string(current))
			current = []rune{r}
		} else {
			current = append(current, r)
		}
	}
	if len(current) > 0 {
		words = append(words, string(current))
	}
	return strings.Join(words, " ")
}

func BuildSummaryFromHandler(name string) string {
	if name == "" {
		return ""
	}
	// Remove package prefix
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		name = name[idx+1:]
	}
	words := strings.ToLower(CameraCaseToWords(name))
	if words == "" {
		return ""
	}
	// Capitalize first letter
	if len(words) > 0 {
		words = strings.ToUpper(words[:1]) + words[1:]
	}
	return words
}

func BuildSummaryFromMethodAndPath(method, path string) string {
	op := strings.ToLower(MethodToOperation(method, path))
	segment := ExtractFirstPathSegment(path)
	if segment == "" {
		return op
	}
	if op == "list" || op == "create" {
		segment = Pluralize(segment)
	} else {
		segment = Singularize(segment)
	}
	return op + " " + segment
}

func BuildDescription(summary string) string {
	if summary == "" {
		return ""
	}
	words := strings.ToLower(summary[:1]) + summary[1:]
	return words[:1] + strings.ToLower(words[:1]) + words[1:] + ""
}

func ExtractTag(path, basePath string) string {
	stripped := StripBasePath(path, basePath)
	segment := ExtractFirstPathSegment(stripped)
	if segment == "" {
		return "default"
	}
	return strings.ToLower(segment)
}

func IsAuthRoute(name, path string) (string, bool) {
	nameLower := strings.ToLower(name)
	pathLower := strings.ToLower(path)

	registerNames := []string{"register", "signup", "sign_up", "registration"}
	loginNames := []string{"login", "signin", "sign_in", "authenticate", "auth"}
	logoutNames := []string{"logout", "signout", "sign_out"}

	registerPaths := []string{"/register", "/signup", "/auth/register"}
	loginPaths := []string{"/login", "/signin", "/auth/login", "/auth/signin"}
	logoutPaths := []string{"/logout", "/auth/logout"}

	for _, n := range registerNames {
		if containsString(nameLower, n) {
			return "register", true
		}
	}
	for _, p := range registerPaths {
		if pathLower == p || strings.HasPrefix(pathLower, p+"/") {
			return "register", true
		}
	}

	for _, n := range loginNames {
		if containsString(nameLower, n) {
			return "login", true
		}
	}
	for _, p := range loginPaths {
		if pathLower == p || strings.HasPrefix(pathLower, p+"/") {
			return "login", true
		}
	}

	for _, n := range logoutNames {
		if containsString(nameLower, n) {
			return "logout", true
		}
	}
	for _, p := range logoutPaths {
		if pathLower == p || strings.HasPrefix(pathLower, p+"/") {
			return "logout", true
		}
	}

	return "", false
}

func DefaultSwaggoErrorFormat(statusCode int) string {
	switch statusCode {
	case 400:
		return "// @Failure 400 {object} ErrorResponse"
	case 401:
		return "// @Failure 401 {object} ErrorResponse"
	case 403:
		return "// @Failure 403 {object} ErrorResponse"
	case 404:
		return "// @Failure 404 {object} ErrorResponse"
	case 409:
		return "// @Failure 409 {object} ErrorResponse"
	case 422:
		return "// @Failure 422 {object} ErrorResponse"
	case 500:
		return "// @Failure 500 {object} ErrorResponse"
	default:
		return fmt.Sprintf("// @Failure %d {object} ErrorResponse", statusCode)
	}
}

func GetSecurityName(acfg *AnalysisConfig) string {
	if acfg != nil && acfg.SecurityName != "" {
		return acfg.SecurityName
	}
	return "BearerAuth"
}

func DetectAuthByHeaderRead(body *ast.BlockStmt, acfg *AnalysisConfig) bool {
	if body == nil {
		return false
	}
	authHeader := "Authorization"
	if acfg != nil && acfg.AuthHeader != "" {
		authHeader = acfg.AuthHeader
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
		if sel.Sel.Name == "Get" {
			if len(call.Args) > 0 {
				if lit, ok := call.Args[0].(*ast.BasicLit); ok {
					if strings.Trim(lit.Value, `"`) == authHeader {
						found = true
						return false
					}
				}
			}
		}
		return true
	})
	return found
}
