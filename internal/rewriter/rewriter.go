package rewriter

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"os"
	"strings"

	"goswaggergen/internal/analyzer"
)

type Rewriter struct {
	DryRun     bool
	Verbose    bool
	VerboseLog func(format string, args ...interface{})
}

func New(dryRun, verbose bool, verboseLog func(string, ...interface{})) *Rewriter {
	return &Rewriter{
		DryRun:     dryRun,
		Verbose:    verbose,
		VerboseLog: verboseLog,
	}
}

func (rw *Rewriter) InsertComments(routes []*analyzer.RouteInfo, commentMap map[*analyzer.RouteInfo]string) error {
	type fileMod struct {
		filePath string
		comments map[int]string
	}

	fileMods := make(map[string]*fileMod)

	for _, route := range routes {
		comment, ok := commentMap[route]
		if !ok || comment == "" {
			continue
		}

		if route.Handler != nil && route.Handler.IsAnon {
			if route.Fset == nil || route.File == nil {
				continue
			}
			filePos := route.Fset.Position(route.File.Pos())
			if filePos.Filename == "" {
				continue
			}
			routePos := route.Fset.Position(route.RouteNode.Pos())
			line := routePos.Line

			if _, ok := fileMods[filePos.Filename]; !ok {
				fileMods[filePos.Filename] = &fileMod{
					filePath: filePos.Filename,
					comments: make(map[int]string),
				}
			}
			fileMods[filePos.Filename].comments[line] = comment
			continue
		}

		var targetFset *token.FileSet
		var targetFile *ast.File
		var handlerFuncDecl *ast.FuncDecl

		if route.Handler != nil && route.Handler.FuncDecl != nil {
			handlerFuncDecl = route.Handler.FuncDecl
			targetFile = route.Handler.File
			targetFset = route.Handler.Fset
		} else {
			continue
		}

		if targetFset == nil || targetFile == nil {
			continue
		}

		pos := targetFset.Position(targetFile.Pos())
		if pos.Filename == "" {
			continue
		}

		if handlerFuncDecl.Doc != nil {
			if hasExistingSwagger(handlerFuncDecl.Doc) {
				continue
			}
		}

		handlerPos := targetFset.Position(handlerFuncDecl.Pos())
		line := handlerPos.Line

		if _, ok := fileMods[pos.Filename]; !ok {
			fileMods[pos.Filename] = &fileMod{
				filePath: pos.Filename,
				comments: make(map[int]string),
			}
		}
		fileMods[pos.Filename].comments[line] = comment
	}

	for _, fm := range fileMods {
		if err := rw.applyToFile(fm.filePath, fm.comments); err != nil {
			return fmt.Errorf("failed to modify %s: %w", fm.filePath, err)
		}
	}

	return nil
}

func (rw *Rewriter) applyToFile(filePath string, lineComments map[int]string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")

	var result []string
	for i, line := range lines {
		lineNum := i + 1
		if comment, ok := lineComments[lineNum]; ok {
			commentLines := strings.Split(comment, "\n")
			result = append(result, commentLines...)
		}
		result = append(result, line)
	}

	finalContent := strings.Join(result, "\n")

		formatted, err := format.Source([]byte(finalContent))
		if err != nil {
			if rw.VerboseLog != nil {
				rw.VerboseLog("warning: go/format failed for %s: %v", filePath, err)
			}
		} else {
		finalContent = string(formatted)
	}

	if rw.DryRun {
		fmt.Println("would write to " + filePath)
		return nil
	}

	info, err := os.Stat(filePath)
	var mode os.FileMode = 0644
	if err == nil {
		mode = info.Mode()
	}

	if err := os.WriteFile(filePath, []byte(finalContent), mode); err != nil {
		return err
	}

	if rw.VerboseLog != nil {
		rw.VerboseLog("wrote swagger comments to %s", filePath)
	}
	return nil
}

func hasExistingSwagger(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	tags := []string{
		"@Summary", "@Description", "@Tags", "@Accept", "@Produce",
		"@Param", "@Success", "@Failure", "@Router", "@Security",
	}
	for _, c := range doc.List {
		for _, tag := range tags {
			if strings.Contains(c.Text, tag) {
				return true
			}
		}
	}
	return false
}
