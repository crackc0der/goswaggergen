package analyzer

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/packages"
)

type RouteInfo struct {
	Method      string
	Path        string
	Handler     *HandlerInfo
	Middleware   []string
	IsProtected bool
	AuthName    string
	RouteNode   ast.Node
	File        *ast.File
	Fset        *token.FileSet
	Pkg         *packages.Package
}

type HandlerInfo struct {
	Name      string
	FuncDecl  *ast.FuncDecl
	File      *ast.File
	Fset      *token.FileSet
	PkgPath   string
	Pkg       *packages.Package
	IsMethod  bool
	Receiver  string
	IsAnon    bool
	CallExpr  *ast.CallExpr
}

type ParamInfo struct {
	Name     string
	In       string
	Type     string
	Required bool
}

type SwaggerComment struct {
	Summary     string
	Description string
	Tags        []string
	Params      []ParamInfo
	Success     []SwaggerResponse
	Failure     []SwaggerResponse
	Security    []string
	Router      string
	Accepts     []string
	Produces    []string
}

type SwaggerResponse struct {
	Code   int
	Type   string
	Schema string
}

type RequestBodyInfo struct {
	TypeName string
	Required bool
}

type Analyzer struct {
	AllPkgs []*packages.Package
	Config   interface{}
}

type AnalysisConfig struct {
	InferRequestBody  bool
	InferResponseBody bool
	InferQueryParams  bool
	InferPathParams   bool
	InferStatusCodes  bool
	InferAuth         bool
	BasePath          string
	DefaultErrorType  string
	AuthHeader        string
	AuthPrefix        string
	SecurityName      string
}
