package loader

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

type FileInfo struct {
	File  *ast.File
	Fset  *token.FileSet
	Pkg   *packages.Package
}

type PackageData struct {
	Package   *packages.Package
	Fset      *token.FileSet
	Files     []*ast.File
	TypesInfo *types.Info
	TypesPkg  *types.Package
}

func LoadPackages(pattern string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedSyntax |
			packages.NeedTypesSizes,
	}
	pkgs, err := packages.Load(cfg, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to load packages: %w", err)
	}
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			for _, e := range pkg.Errors {
				if e.Kind == packages.ListError {
					return nil, fmt.Errorf("package load error: %v", e)
				}
			}
		}
	}
	return pkgs, nil
}

func IsChiImport(pkg *packages.Package) bool {
	for path := range pkg.Imports {
		if path == "github.com/go-chi/chi/v5" || path == "github.com/go-chi/chi" {
			return true
		}
	}
	return false
}

func FindChiImportPath(pkg *packages.Package) string {
	for path := range pkg.Imports {
		if path == "github.com/go-chi/chi/v5" {
			return path
		}
		if path == "github.com/go-chi/chi" {
			return path
		}
	}
	return ""
}

func GetChiPackageName(pkg *packages.Package) string {
	for path, imp := range pkg.Imports {
		if path == "github.com/go-chi/chi/v5" || path == "github.com/go-chi/chi" {
			return imp.Name
		}
	}
	return "chi"
}

func HasChiRouter(pkg *packages.Package) bool {
	return IsChiImport(pkg)
}
