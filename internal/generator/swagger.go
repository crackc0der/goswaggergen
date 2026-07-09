package generator

import (
	"fmt"
	"go/ast"
	"sort"
	"strings"

	"goswaggergen/internal/analyzer"
)

type SwaggerGen struct {
	Config *analyzer.AnalysisConfig
}

func NewSwaggerGen(config *analyzer.AnalysisConfig) *SwaggerGen {
	return &SwaggerGen{Config: config}
}

func (sg *SwaggerGen) GenerateComment(route *analyzer.RouteInfo, bodyAnalysis *analyzer.HandlerAnalysisResult, authResult *analyzer.AuthResult) string {
	var lines []string

	handlerName := sg.handlerDisplayName(route)
	summary := sg.buildSummary(route, authResult)
	description := sg.buildDescription(route, authResult)
	tag := sg.buildTag(route, authResult)

	lines = append(lines, fmt.Sprintf("// %s godoc", handlerName))
	lines = append(lines, fmt.Sprintf("// @Summary %s", summary))
	lines = append(lines, fmt.Sprintf("// @Description %s", description))
	lines = append(lines, fmt.Sprintf("// @Tags %s", tag))

	hasBody := bodyAnalysis.RequestBody != nil && bodyAnalysis.RequestBody.TypeName != ""
	if hasBody {
		lines = append(lines, "// @Accept json")
	}
	lines = append(lines, "// @Produce json")

	// Path params
	pathParams := analyzer.ExtractPathParams(route.Path)
	for _, pp := range pathParams {
		paramType := guessPathParamType(pp)
		required := "true"
		for _, bp := range bodyAnalysis.PathParams {
			if bp.Name == pp {
				paramType = bp.Type
				break
			}
		}
		lines = append(lines, fmt.Sprintf("// @Param %s path %s %s \"%s\"", pp, paramType, required, pp))
	}

	// Request body param
	if bodyAnalysis.RequestBody != nil && bodyAnalysis.RequestBody.TypeName != "" {
		desc := "Request body"
		if authResult.IsRegister {
			desc = "Register request"
		} else if authResult.IsLogin {
			desc = "Login request"
		}
		lines = append(lines, fmt.Sprintf("// @Param request body %s true \"%s\"", bodyAnalysis.RequestBody.TypeName, desc))
	}

	// Query params
	for _, qp := range bodyAnalysis.QueryParams {
		required := "false"
		if qp.Required {
			required = "true"
		}
		paramType := qp.Type
		if paramType == "string" {
			paramType = guessQueryParamType(qp.Name)
		}
		lines = append(lines, fmt.Sprintf("// @Param %s query %s %s \"%s\"", qp.Name, paramType, required, qp.Name))
	}

	// Header params (skip Authorization)
	for _, hp := range bodyAnalysis.HeaderParams {
		if strings.EqualFold(hp.Name, "Authorization") || strings.EqualFold(hp.Name, "authorization") {
			continue
		}
		required := "false"
		if hp.Required {
			required = "true"
		}
		lines = append(lines, fmt.Sprintf("// @Param %s header %s %s \"%s\"", hp.Name, hp.Type, required, hp.Name))
	}

	// Success responses
	successCode := sg.getSuccessCode(route, bodyAnalysis)
	sg.addSuccessLines(&lines, bodyAnalysis, successCode, authResult)

	// Failure responses
	failureCodes := sg.collectFailureCodes(route, bodyAnalysis, authResult)
	sg.addFailureLines(&lines, failureCodes, bodyAnalysis)

	// Security
	if authResult.IsProtected || route.IsProtected {
		secName := authResult.SecurityName
		if secName == "" {
			secName = sg.Config.SecurityName
		}
		if secName == "" {
			secName = "BearerAuth"
		}
		lines = append(lines, fmt.Sprintf("// @Security %s", secName))
	}

	// Router
	lines = append(lines, fmt.Sprintf("// @Router %s [%s]", route.Path, strings.ToLower(route.Method)))

	return strings.Join(lines, "\n")
}

func (sg *SwaggerGen) handlerDisplayName(route *analyzer.RouteInfo) string {
	if route.Handler != nil && route.Handler.Name != "" {
		parts := strings.Split(route.Handler.Name, ".")
		return parts[len(parts)-1]
	}
	stripped := analyzer.StripBasePath(route.Path, sg.Config.BasePath)
	seg := analyzer.ExtractFirstPathSegment(stripped)
	if seg != "" {
		seg = strings.ToUpper(seg[:1]) + seg[1:]
		return seg
	}
	return analyzer.BuildSummaryFromMethodAndPath(route.Method, stripped)
}

func (sg *SwaggerGen) buildSummary(route *analyzer.RouteInfo, authResult *analyzer.AuthResult) string {
	if authResult.IsRegister {
		return "Register user"
	}
	if authResult.IsLogin {
		return "Login user"
	}
	if authResult.IsLogout {
		return "Logout user"
	}

	if route.Handler != nil && route.Handler.Name != "" {
		summary := analyzer.BuildSummaryFromHandler(route.Handler.Name)
		if summary != "" {
			return summary
		}
	}

	stripped := analyzer.StripBasePath(route.Path, sg.Config.BasePath)
	if route.Handler != nil && route.Handler.IsAnon && !strings.Contains(stripped[1:], "/") && !strings.Contains(stripped, "{") {
		seg := analyzer.ExtractFirstPathSegment(stripped)
		return strings.ToUpper(seg[:1]) + seg[1:]
	}
	return analyzer.BuildSummaryFromMethodAndPath(route.Method, stripped)
}

func (sg *SwaggerGen) buildDescription(route *analyzer.RouteInfo, authResult *analyzer.AuthResult) string {
	if authResult.IsRegister {
		return "Register a new user account"
	}
	if authResult.IsLogin {
		return "Authenticate user and return access token"
	}
	if authResult.IsLogout {
		return "Logout user"
	}

	stripped := analyzer.StripBasePath(route.Path, sg.Config.BasePath)
	if route.Handler != nil && route.Handler.IsAnon && !strings.Contains(stripped[1:], "/") && !strings.Contains(stripped, "{") {
		seg := analyzer.ExtractFirstPathSegment(stripped)
		return strings.ToLower(seg[:1]) + seg[1:]
	}

	summary := sg.buildSummary(route, authResult)
	if summary == "" {
		return ""
	}
	return strings.ToLower(summary[:1]) + summary[1:]
}

func (sg *SwaggerGen) buildTag(route *analyzer.RouteInfo, authResult *analyzer.AuthResult) string {
	if authResult.IsRegister || authResult.IsLogin || authResult.IsLogout {
		return "auth"
	}

	tag := analyzer.ExtractTag(route.Path, sg.Config.BasePath)
	if tag == "" {
		return "default"
	}
	return tag
}

func (sg *SwaggerGen) getSuccessCode(route *analyzer.RouteInfo, bodyAnalysis *analyzer.HandlerAnalysisResult) int {
	if bodyAnalysis.ResponseBody != nil && bodyAnalysis.ResponseBody.StatusCode > 0 {
		return bodyAnalysis.ResponseBody.StatusCode
	}
	for code := range bodyAnalysis.AllStatusCodes {
		if code < 400 {
			return code
		}
	}
	return analyzer.DefaultStatusCode(route.Method)
}

func (sg *SwaggerGen) addSuccessLines(lines *[]string, bodyAnalysis *analyzer.HandlerAnalysisResult, defaultCode int, authResult *analyzer.AuthResult) {
	if bodyAnalysis.ResponseBody != nil && bodyAnalysis.ResponseBody.TypeName != "" {
		code := bodyAnalysis.ResponseBody.StatusCode
		if code == 0 {
			code = defaultCode
		}
		typeName := bodyAnalysis.ResponseBody.TypeName
		if bodyAnalysis.ResponseBody.IsArray {
			typeName = strings.TrimPrefix(typeName, "[]")
			*lines = append(*lines, fmt.Sprintf("// @Success %d {array} %s", code, typeName))
		} else if bodyAnalysis.ResponseBody.IsMap {
			typeName = strings.TrimPrefix(typeName, "map")
			*lines = append(*lines, fmt.Sprintf("// @Success %d {object} %s", code, typeName))
		} else {
			*lines = append(*lines, fmt.Sprintf("// @Success %d {object} %s", code, typeName))
		}
	} else {
		// Check if there's any success code found
		hasSuccess := false
		for code := range bodyAnalysis.AllStatusCodes {
			if code < 400 {
				hasSuccess = true
				break
			}
		}
		if !hasSuccess {
			if defaultCode == 204 {
				*lines = append(*lines, fmt.Sprintf("// @Success %d", defaultCode))
			} else {
				// Default success response
				if authResult.IsRegister {
					*lines = append(*lines, fmt.Sprintf("// @Success %d {object} dto.AuthResponse", defaultCode))
				} else if authResult.IsLogin {
					*lines = append(*lines, fmt.Sprintf("// @Success %d {object} dto.AuthResponse", defaultCode))
				} else {
					*lines = append(*lines, fmt.Sprintf("// @Success %d", defaultCode))
				}
			}
		} else {
			for code := range bodyAnalysis.AllStatusCodes {
				if code < 400 {
					*lines = append(*lines, fmt.Sprintf("// @Success %d", code))
					break
				}
			}
		}
	}
}

func (sg *SwaggerGen) collectFailureCodes(route *analyzer.RouteInfo, bodyAnalysis *analyzer.HandlerAnalysisResult, authResult *analyzer.AuthResult) []ErrorInfo {
	var errors []ErrorInfo
	seen := make(map[int]bool)

	for _, er := range bodyAnalysis.ErrorResponses {
		if !seen[er.StatusCode] {
			seen[er.StatusCode] = true
			errors = append(errors, ErrorInfo{Code: er.StatusCode, TypeName: er.TypeName})
		}
	}

	for code := range bodyAnalysis.AllStatusCodes {
		if code >= 400 && !seen[code] {
			seen[code] = true
			errType := sg.Config.DefaultErrorType
			if errType == "" {
				errType = "ErrorResponse"
			}
			errType = sg.stripQuotes(errType)
			errors = append(errors, ErrorInfo{Code: code, TypeName: errType})
		}
	}

	sort.Slice(errors, func(i, j int) bool {
		return errors[i].Code < errors[j].Code
	})

	return errors
}

func (sg *SwaggerGen) getErrorType() string {
	errType := sg.Config.DefaultErrorType
	if errType == "" {
		errType = "ErrorResponse"
	}
	return sg.stripQuotes(errType)
}

func (sg *SwaggerGen) stripQuotes(s string) string {
	return strings.Trim(s, `"`)
}

type ErrorInfo struct {
	Code     int
	TypeName string
}

func (sg *SwaggerGen) addFailureLines(lines *[]string, errors []ErrorInfo, bodyAnalysis *analyzer.HandlerAnalysisResult) {
	for _, e := range errors {
		typeName := e.TypeName
		if typeName == "" {
			typeName = sg.getErrorType()
		}
		if typeName != "" {
			*lines = append(*lines, fmt.Sprintf("// @Failure %d {object} %s", e.Code, typeName))
		} else {
			*lines = append(*lines, fmt.Sprintf("// @Failure %d", e.Code))
		}
	}
}

func HasExistingSwagger(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	swaggerTags := []string{
		"@Summary", "@Description", "@Tags", "@Accept", "@Produce",
		"@Param", "@Success", "@Failure", "@Router", "@Security",
	}
	for _, comment := range doc.List {
		text := comment.Text
		for _, tag := range swaggerTags {
			if strings.Contains(text, tag) {
				return true
			}
		}
	}
	return false
}

func guessPathParamType(name string) string {
	lower := strings.ToLower(name)
	if strings.Contains(lower, "id") {
		return "int"
	}
	return "string"
}

func guessQueryParamType(name string) string {
	lower := strings.ToLower(name)
	numericNames := map[string]bool{
		"page": true, "limit": true, "offset": true, "size": true,
		"count": true, "per_page": true, "perpage": true, "skip": true,
	}
	if numericNames[lower] {
		return "int"
	}
	return "string"
}
