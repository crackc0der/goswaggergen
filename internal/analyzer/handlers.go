package analyzer

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

type BodyAnalyzer struct {
	Acfg       *AnalysisConfig
	ChiPkgName string
	VerboseLog func(format string, args ...interface{})
}

func NewBodyAnalyzer(acfg *AnalysisConfig, chiPkgName string, verboseLog func(string, ...interface{})) *BodyAnalyzer {
	return &BodyAnalyzer{
		Acfg:       acfg,
		ChiPkgName: chiPkgName,
		VerboseLog: verboseLog,
	}
}

type HandlerAnalysisResult struct {
	RequestBody    *RequestBodyInfo
	ResponseBody   *ResponseBodyInfo
	ErrorResponses []ErrorResponseInfo
	PathParams     []ParamInfo
	QueryParams    []ParamInfo
	HeaderParams   []ParamInfo
	AllStatusCodes map[int]bool
	HasAuth        bool
}

type ResponseBodyInfo struct {
	TypeName   string
	StatusCode int
	IsArray    bool
	IsMap      bool
}

type ErrorResponseInfo struct {
	StatusCode int
	TypeName   string
}

func (ba *BodyAnalyzer) AnalyzeHandler(handler *HandlerInfo) *HandlerAnalysisResult {
	result := &HandlerAnalysisResult{
		AllStatusCodes: make(map[int]bool),
	}

	if handler.FuncDecl == nil || handler.FuncDecl.Body == nil {
		return result
	}

	body := handler.FuncDecl.Body
	varTypes := ba.buildVariableTypes(body)

	if ba.Acfg.InferRequestBody {
		result.RequestBody = ba.findRequestBody(body, varTypes)
	}

	if ba.Acfg.InferResponseBody {
		result.ResponseBody = ba.findResponseBody(body, varTypes, result)
	}

	if ba.Acfg.InferPathParams {
		result.PathParams = ba.findPathParams(body, varTypes)
	}

	if ba.Acfg.InferQueryParams {
		result.QueryParams = ba.findQueryParams(body)
	}

	result.HeaderParams = ba.findHeaderParams(body)

	ba.findAllStatusCodes(body, result)

	result.ErrorResponses = ba.findErrorResponses(body, varTypes)

	if ba.Acfg.InferAuth {
		result.HasAuth = DetectAuthByHeaderRead(body, ba.Acfg)
	}

	return result
}

func (ba *BodyAnalyzer) buildVariableTypes(body *ast.BlockStmt) map[string]string {
	varTypes := make(map[string]string)

	if body == nil {
		return varTypes
	}

	ast.Inspect(body, func(n ast.Node) bool {
		// var x Type
		declStmt, ok := n.(*ast.DeclStmt)
		if ok {
			genDecl, ok2 := declStmt.Decl.(*ast.GenDecl)
			if ok2 {
				for _, spec := range genDecl.Specs {
					valSpec, ok3 := spec.(*ast.ValueSpec)
					if ok3 {
						typeName := ba.exprTypeName(valSpec.Type)
						for _, name := range valSpec.Names {
							varTypes[name.Name] = typeName
						}
					}
				}
			}
		}

		// x := SomeType{} or x := NewSomeType() or x := fn()
		assignStmt, ok2 := n.(*ast.AssignStmt)
		if ok2 && assignStmt.Tok == token.DEFINE {
			for i, lhs := range assignStmt.Lhs {
				varName := ba.exprToString(lhs)
				if varName == "" || varName == "_" {
					continue
				}
				if i < len(assignStmt.Rhs) {
					typeName := ba.inferTypeFromRHS(assignStmt.Rhs[i])
					if typeName != "" {
						varTypes[varName] = typeName
					}
				}
			}
		}

		return true
	})

	return varTypes
}

func (ba *BodyAnalyzer) resolveType(expr ast.Expr, varTypes map[string]string) string {
	typeName := ba.exprTypeName(expr)
	if typeName == "" {
		return ""
	}

	// If typeName is just an identifier, try to look it up
	if !strings.Contains(typeName, ".") {
		if resolved, ok := varTypes[typeName]; ok && resolved != "" {
			return resolved
		}
	}

	// It might still be a simple type name (struct in same file)
	return typeName
}

func (ba *BodyAnalyzer) findRequestBody(body *ast.BlockStmt, varTypes map[string]string) *RequestBodyInfo {
	var result *RequestBodyInfo

	ast.Inspect(body, func(n ast.Node) bool {
		if result != nil {
			return false
		}

		declStmt, ok := n.(*ast.DeclStmt)
		if ok {
			genDecl, ok2 := declStmt.Decl.(*ast.GenDecl)
			if ok2 && len(genDecl.Specs) > 0 {
				valSpec, ok3 := genDecl.Specs[0].(*ast.ValueSpec)
				if ok3 && len(valSpec.Names) > 0 && len(valSpec.Values) == 0 {
					typeName := ba.exprTypeName(valSpec.Type)
					if typeName != "" {
						varName := valSpec.Names[0].Name
						if ba.hasDecodeCall(body, varName) {
							result = &RequestBodyInfo{TypeName: typeName, Required: true}
							return false
						}
					}
				}
			}
		}

		assignStmt, ok2 := n.(*ast.AssignStmt)
		if ok2 && assignStmt.Tok == token.DEFINE && len(assignStmt.Lhs) > 0 && len(assignStmt.Rhs) > 0 {
			rhs := assignStmt.Rhs[0]
			lhs := assignStmt.Lhs[0]
			varName := ba.exprToString(lhs)
			typeName := ba.inferTypeFromRHS(rhs)
			if typeName != "" && varName != "" {
				if ba.hasDecodeCall(body, varName) {
					result = &RequestBodyInfo{TypeName: typeName, Required: true}
					return false
				}
			}

			// req := new(SomeType)
			if call, ok4 := rhs.(*ast.CallExpr); ok4 {
				if ident, ok5 := call.Fun.(*ast.Ident); ok5 && ident.Name == "new" && len(call.Args) > 0 {
					typeName = ba.exprTypeName(call.Args[0])
					if typeName != "" && varName != "" {
						if ba.hasDecodeCall(body, varName) {
							result = &RequestBodyInfo{TypeName: typeName, Required: true}
							return false
						}
					}
				}
			}
		}

		// body, _ := io.ReadAll(r.Body); json.Unmarshal(body, &req)
		if assignStmt3, ok4 := n.(*ast.AssignStmt); ok4 && assignStmt3.Tok == token.DEFINE && len(assignStmt3.Lhs) > 0 && len(assignStmt3.Rhs) > 0 {
			lhs := assignStmt3.Lhs[0]
			bodyVar := ba.exprToString(lhs)
			rhs := assignStmt3.Rhs[0]
			if call, ok5 := rhs.(*ast.CallExpr); ok5 {
				if ba.isCallToFunc(call, "io", "ReadAll") || ba.isCallToFunc(call, "", "ReadAll") {
					varType := ba.findUnmarshalTarget(body, bodyVar, varTypes)
					if varType != "" {
						result = &RequestBodyInfo{TypeName: varType, Required: true}
						return false
					}
				}
			}
		}

		return true
	})

	return result
}

func (ba *BodyAnalyzer) hasDecodeCall(body *ast.BlockStmt, varName string) bool {
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
		if sel.Sel.Name == "Decode" && len(call.Args) > 0 {
			arg := call.Args[0]
			if unary, ok := arg.(*ast.UnaryExpr); ok && unary.Op == token.AND {
				if ident, ok := unary.X.(*ast.Ident); ok && ident.Name == varName {
					found = true
					return false
				}
			}
			if ident, ok := arg.(*ast.Ident); ok && ident.Name == varName {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func (ba *BodyAnalyzer) findUnmarshalTarget(body *ast.BlockStmt, bodyVar string, varTypes map[string]string) string {
	var targetType string
	ast.Inspect(body, func(n ast.Node) bool {
		if targetType != "" {
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
		if sel.Sel.Name == "Unmarshal" && len(call.Args) >= 2 {
			if ident, ok := call.Args[0].(*ast.Ident); ok && ident.Name == bodyVar {
				arg := call.Args[1]
				if unary, ok := arg.(*ast.UnaryExpr); ok && unary.Op == token.AND {
					if ident2, ok := unary.X.(*ast.Ident); ok {
						targetType = varTypes[ident2.Name]
					}
				}
				return false
			}
		}
		return true
	})
	return targetType
}

func (ba *BodyAnalyzer) findResponseBody(body *ast.BlockStmt, varTypes map[string]string, result *HandlerAnalysisResult) *ResponseBodyInfo {
	var resp *ResponseBodyInfo

	ast.Inspect(body, func(n ast.Node) bool {
		if resp != nil {
			return false
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			// Check for writeJSON(w, code, resp)
			if ident, ok2 := call.Fun.(*ast.Ident); ok2 {
				if (ident.Name == "writeJSON" || ident.Name == "WriteJSON" || ident.Name == "respond") && len(call.Args) >= 3 {
					code := ba.extractStatusCode(call.Args[1])
					if code > 0 && code < 600 {
						typeName := ba.resolveType(call.Args[2], varTypes)
						if typeName != "" && !ba.isErrorType(typeName) {
							resp = &ResponseBodyInfo{TypeName: typeName, StatusCode: code}
							if ba.isSliceExpr(call.Args[2], varTypes) {
								resp.IsArray = true
							}
							if ba.isMapExpr(call.Args[2], varTypes) {
								resp.IsMap = true
							}
							return false
						}
					}
				}
			}
			return true
		}

		// Pattern: json.NewEncoder(w).Encode(resp)
		if sel.Sel.Name == "Encode" && len(call.Args) > 0 {
			if _, ok2 := sel.X.(*ast.CallExpr); ok2 {
				typeName := ba.resolveType(call.Args[0], varTypes)
				if typeName != "" && !ba.isErrorType(typeName) {
					code := 200
					resp = &ResponseBodyInfo{TypeName: typeName, StatusCode: code}
					if ba.isSliceExpr(call.Args[0], varTypes) {
						resp.IsArray = true
					}
					if ba.isMapExpr(call.Args[0], varTypes) {
						resp.IsMap = true
					}
					return false
				}
			}
		}

		// Pattern: render.JSON(w, r, resp) or response.JSON(w, code, resp) or h.respond(w, code, resp)
		if (sel.Sel.Name == "JSON" || sel.Sel.Name == "respond" || sel.Sel.Name == "Respond" || sel.Sel.Name == "RespondJSON" || sel.Sel.Name == "writeJSON") && len(call.Args) >= 2 {
			var codeExpr ast.Expr
			var dataExpr ast.Expr

			if len(call.Args) >= 3 {
				codeExpr = call.Args[1]
				dataExpr = call.Args[2]
			} else if len(call.Args) == 2 {
				codeExpr = nil
				dataExpr = call.Args[1]
			}

			code := 200
			if codeExpr != nil {
				if c := ba.extractStatusCode(codeExpr); c > 0 {
					code = c
				}
			}

			typeName := ba.resolveType(dataExpr, varTypes)
			if typeName != "" && !ba.isErrorType(typeName) {
				resp = &ResponseBodyInfo{TypeName: typeName, StatusCode: code}
				if ba.isSliceExpr(dataExpr, varTypes) {
					resp.IsArray = true
				}
				if ba.isMapExpr(dataExpr, varTypes) {
					resp.IsMap = true
				}
				return false
			}
		}

		return true
	})

	return resp
}

func (ba *BodyAnalyzer) findErrorResponses(body *ast.BlockStmt, varTypes map[string]string) []ErrorResponseInfo {
	var errors []ErrorResponseInfo
	seen := make(map[int]bool)

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if ok {
			// http.Error(w, msg, code)
			if xid, ok2 := sel.X.(*ast.Ident); ok2 && xid.Name == "http" && sel.Sel.Name == "Error" {
				if len(call.Args) >= 3 {
					code := ba.extractStatusCode(call.Args[2])
					if code > 0 && !seen[code] {
						seen[code] = true
						errType := ba.Acfg.DefaultErrorType
						if errType == "" {
							errType = "ErrorResponse"
						}
						errors = append(errors, ErrorResponseInfo{StatusCode: code, TypeName: errType})
					}
				}
			}
		}

		// writeJSON(w, code, ErrorResponse{...})
		if ident, ok3 := call.Fun.(*ast.Ident); ok3 {
			if (ident.Name == "writeJSON" || ident.Name == "WriteJSON" || ident.Name == "respond") && len(call.Args) >= 3 {
				code := ba.extractStatusCode(call.Args[1])
				if code > 0 && code >= 400 {
					typeName := ba.resolveType(call.Args[2], varTypes)
					if typeName != "" && !seen[code] {
						seen[code] = true
						if ba.isErrorType(typeName) || code >= 400 {
							errType := typeName
							if !ba.isErrorType(typeName) {
								errType = ba.Acfg.DefaultErrorType
								if errType == "" {
									errType = "ErrorResponse"
								}
							}
							errors = append(errors, ErrorResponseInfo{StatusCode: code, TypeName: errType})
						}
					}
				}
			}
		}

		// response.JSON/respond pattern
		if ok {
			if sel.Sel.Name == "JSON" || sel.Sel.Name == "respond" || sel.Sel.Name == "Respond" || sel.Sel.Name == "RespondJSON" || sel.Sel.Name == "writeJSON" {
				if len(call.Args) >= 3 {
					code := ba.extractStatusCode(call.Args[1])
					if code > 0 && code >= 400 {
						typeName := ba.resolveType(call.Args[2], varTypes)
						if !seen[code] {
							seen[code] = true
							errType := typeName
							if !ba.isErrorType(typeName) {
								errType = ba.Acfg.DefaultErrorType
								if errType == "" {
									errType = "ErrorResponse"
								}
							}
							errors = append(errors, ErrorResponseInfo{StatusCode: code, TypeName: errType})
						}
					}
				}
			}
		}

		return true
	})

	return errors
}

func (ba *BodyAnalyzer) findPathParams(body *ast.BlockStmt, varTypes map[string]string) []ParamInfo {
	var params []ParamInfo
	seen := make(map[string]bool)

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		if sel.Sel.Name == "URLParam" {
			if xid, ok2 := sel.X.(*ast.Ident); ok2 && (xid.Name == ba.ChiPkgName || xid.Name == "chi") {
				if len(call.Args) >= 2 {
					paramName := ba.extractStringArg(call.Args[1])
					if paramName != "" && !seen[paramName] {
						seen[paramName] = true
						paramType := "string"
						if ba.isIntConversionCall(body, paramName) {
							paramType = "int"
						}
						params = append(params, ParamInfo{
							Name:     paramName,
							In:       "path",
							Type:     paramType,
							Required: true,
						})
					}
				}
			}
		}

		return true
	})

	return params
}

func (ba *BodyAnalyzer) findQueryParams(body *ast.BlockStmt) []ParamInfo {
	var params []ParamInfo
	seen := make(map[string]bool)

	ast.Inspect(body, func(n ast.Node) bool {
		assignStmt, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}

		for _, rhs := range assignStmt.Rhs {
			paramName, paramType := ba.extractQueryGet(rhs)
			if paramName != "" && !seen[paramName] {
				seen[paramName] = true
				required := ba.isRequiredParam(body, paramName)
				params = append(params, ParamInfo{
					Name:     paramName,
					In:       "query",
					Type:     paramType,
					Required: required,
				})
			}
		}

		return true
	})

	return params
}

func (ba *BodyAnalyzer) extractQueryGet(expr ast.Expr) (string, string) {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return "", ""
	}

	// Check if it's Atoi wrapping
	if ident, ok3 := call.Fun.(*ast.Ident); ok3 && ident.Name == "Atoi" && len(call.Args) > 0 {
		name, _ := ba.extractQueryGet(call.Args[0])
		if name != "" {
			return name, "int"
		}
		return "", ""
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", ""
	}

	if sel.Sel.Name != "Get" || len(call.Args) == 0 {
		return "", ""
	}

	callSel, ok2 := sel.X.(*ast.CallExpr)
	if !ok2 {
		return "", ""
	}

	querySel, ok3 := callSel.Fun.(*ast.SelectorExpr)
	if !ok3 || querySel.Sel.Name != "Query" {
		return "", ""
	}

	urlSel, ok4 := querySel.X.(*ast.SelectorExpr)
	if !ok4 || urlSel.Sel.Name != "URL" {
		return "", ""
	}

	paramName := ba.extractStringArg(call.Args[0])
	if paramName != "" {
		return paramName, "string"
	}

	return "", ""
}

func (ba *BodyAnalyzer) isIntConversionCall(body *ast.BlockStmt, paramName string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if sel.Sel.Name == "Atoi" && len(call.Args) > 0 {
				innerCall, ok := call.Args[0].(*ast.CallExpr)
				if ok {
					if innerSel, ok2 := innerCall.Fun.(*ast.SelectorExpr); ok2 {
						if innerSel.Sel.Name == "URLParam" && len(innerCall.Args) >= 2 {
							if ba.extractStringArg(innerCall.Args[1]) == paramName {
								found = true
								return false
							}
						}
					}
				}
			}
		}
		if ident, ok := call.Fun.(*ast.Ident); ok {
			if ident.Name == "Atoi" && len(call.Args) > 0 {
				innerCall, ok := call.Args[0].(*ast.CallExpr)
				if ok {
					if innerSel, ok2 := innerCall.Fun.(*ast.SelectorExpr); ok2 {
						if innerSel.Sel.Name == "URLParam" && len(innerCall.Args) >= 2 {
							if ba.extractStringArg(innerCall.Args[1]) == paramName {
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

func (ba *BodyAnalyzer) isRequiredParam(body *ast.BlockStmt, paramName string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		if bin, ok := ifStmt.Cond.(*ast.BinaryExpr); ok {
			if bin.Op == token.EQL {
				if ident, ok := bin.X.(*ast.Ident); ok && ident.Name == paramName {
					if lit, ok := bin.Y.(*ast.BasicLit); ok && lit.Value == `""` {
						// Check if body contains http.Error or similar
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

func (ba *BodyAnalyzer) findHeaderParams(body *ast.BlockStmt) []ParamInfo {
	var params []ParamInfo
	seen := make(map[string]bool)

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		if sel.Sel.Name == "Get" {
			callSel, ok2 := sel.X.(*ast.CallExpr)
			if !ok2 {
				return true
			}
			headerSel, ok3 := callSel.Fun.(*ast.SelectorExpr)
			if !ok3 || headerSel.Sel.Name != "Header" {
				return true
			}
			if len(call.Args) > 0 {
				paramName := ba.extractStringArg(call.Args[0])
				if paramName != "" && !seen[paramName] {
					seen[paramName] = true
					params = append(params, ParamInfo{
						Name:     paramName,
						In:       "header",
						Type:     "string",
						Required: false,
					})
				}
			}
		}

		return true
	})

	return params
}

func (ba *BodyAnalyzer) findAllStatusCodes(body *ast.BlockStmt, result *HandlerAnalysisResult) {
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if ok && sel.Sel.Name == "WriteHeader" && len(call.Args) > 0 {
			code := ba.extractStatusCode(call.Args[0])
			if code > 0 {
				result.AllStatusCodes[code] = true
			}
		}

		// http.Error(w, msg, code)
		if sel, ok2 := call.Fun.(*ast.SelectorExpr); ok2 {
			if xid, ok3 := sel.X.(*ast.Ident); ok3 && xid.Name == "http" && sel.Sel.Name == "Error" {
				if len(call.Args) >= 3 {
					code := ba.extractStatusCode(call.Args[2])
					if code > 0 {
						result.AllStatusCodes[code] = true
					}
				}
			}
		}

		// writeJSON(w, code, ...)
		if ident, ok4 := call.Fun.(*ast.Ident); ok4 {
			if ident.Name == "writeJSON" || ident.Name == "WriteJSON" || ident.Name == "respond" {
				if len(call.Args) >= 2 {
					code := ba.extractStatusCode(call.Args[1])
					if code > 0 {
						result.AllStatusCodes[code] = true
					}
				}
			}
		}

		return true
	})
}

func (ba *BodyAnalyzer) extractStatusCode(expr ast.Expr) int {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.INT {
			val, err := strconv.Atoi(e.Value)
			if err == nil {
				return val
			}
		}
	case *ast.SelectorExpr:
		if xid, ok := e.X.(*ast.Ident); ok {
			if xid.Name == "http" {
				return statusCodeFromName(e.Sel.Name)
			}
		}
	case *ast.Ident:
		return statusCodeFromName(e.Name)
	}
	return 0
}

func statusCodeFromName(name string) int {
	switch name {
	case "StatusOK":
		return 200
	case "StatusCreated":
		return 201
	case "StatusAccepted":
		return 202
	case "StatusNoContent":
		return 204
	case "StatusMovedPermanently":
		return 301
	case "StatusFound":
		return 302
	case "StatusBadRequest":
		return 400
	case "StatusUnauthorized":
		return 401
	case "StatusPaymentRequired":
		return 402
	case "StatusForbidden":
		return 403
	case "StatusNotFound":
		return 404
	case "StatusMethodNotAllowed":
		return 405
	case "StatusConflict":
		return 409
	case "StatusGone":
		return 410
	case "StatusUnprocessableEntity":
		return 422
	case "StatusTooManyRequests":
		return 429
	case "StatusInternalServerError":
		return 500
	case "StatusNotImplemented":
		return 501
	case "StatusBadGateway":
		return 502
	case "StatusServiceUnavailable":
		return 503
	default:
		return 0
	}
}

func (ba *BodyAnalyzer) exprTypeName(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return ba.exprToString(e.X) + "." + e.Sel.Name
	case *ast.CompositeLit:
		return ba.exprTypeName(e.Type)
	case *ast.CallExpr:
		return ba.exprTypeName(e.Fun)
	case *ast.StarExpr:
		return ba.exprTypeName(e.X)
	case *ast.ArrayType:
		if elt, ok := e.Elt.(*ast.Ident); ok {
			return "[]" + elt.Name
		}
		if sel, ok := e.Elt.(*ast.SelectorExpr); ok {
			return "[]" + ba.exprToString(sel.X) + "." + sel.Sel.Name
		}
		return ""
	case *ast.MapType:
		return "map[string]string"
	default:
		return ""
	}
}

func (ba *BodyAnalyzer) exprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return ba.exprToString(e.X) + "." + e.Sel.Name
	default:
		return ""
	}
}

func (ba *BodyAnalyzer) inferTypeFromRHS(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.CompositeLit:
		return ba.exprTypeName(e.Type)
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			return ba.exprToString(sel.X) + "." + sel.Sel.Name
		}
		if ident, ok := e.Fun.(*ast.Ident); ok {
			return ident.Name
		}
		return ""
	default:
		return ""
	}
}

func (ba *BodyAnalyzer) isSliceExpr(expr ast.Expr, varTypes map[string]string) bool {
	// Direct array type
	if _, ok := expr.(*ast.ArrayType); ok {
		return true
	}
	// Check if it's a composite literal of array type
	if comp, ok := expr.(*ast.CompositeLit); ok {
		if _, ok2 := comp.Type.(*ast.ArrayType); ok2 {
			return true
		}
	}
	// Check if the variable type starts with []
	if ident, ok := expr.(*ast.Ident); ok {
		typeName, exists := varTypes[ident.Name]
		if exists && strings.HasPrefix(typeName, "[]") {
			return true
		}
	}
	return false
}

func (ba *BodyAnalyzer) isMapExpr(expr ast.Expr, varTypes map[string]string) bool {
	if _, ok := expr.(*ast.MapType); ok {
		return true
	}
	return false
}

func (ba *BodyAnalyzer) isErrorType(typeName string) bool {
	return containsString(strings.ToLower(typeName), "error")
}

func (ba *BodyAnalyzer) extractStringArg(expr ast.Expr) string {
	if lit, ok := expr.(*ast.BasicLit); ok {
		return strings.Trim(lit.Value, `"`)
	}
	return ""
}

func (ba *BodyAnalyzer) isCallToFunc(call *ast.CallExpr, pkg, name string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if pkg != "" {
		if xid, ok := sel.X.(*ast.Ident); ok && xid.Name == pkg {
			return sel.Sel.Name == name
		}
		return false
	}
	return sel.Sel.Name == name
}

func GetFailureCodes(method string) []int {
	switch method {
	case "POST":
		return []int{400, 409, 500}
	case "DELETE":
		return []int{400, 404, 500}
	default:
		return []int{400, 404, 500}
	}
}
