package main

import (
	"flag"
	"fmt"
	"os"

	"goswaggergen/internal/analyzer"
	"goswaggergen/internal/config"
	"goswaggergen/internal/generator"
	"goswaggergen/internal/loader"
	"goswaggergen/internal/report"
	"goswaggergen/internal/rewriter"
)

func main() {
	input := flag.String("input", "", "Path to file, directory, or Go package pattern")
	write := flag.Bool("write", false, "Write changes to files on disk")
	dryRun := flag.Bool("dry-run", false, "Show what would be changed without writing")
	configPath := flag.String("config", "", "Path to YAML configuration file")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	failOnExisting := flag.Bool("fail-on-existing-swagger-change", false, "Fail if existing swagger would be modified")

	flag.Parse()

	if *input == "" {
		fmt.Fprintln(os.Stderr, "error: --input is required")
		flag.Usage()
		os.Exit(1)
	}

	if *write && *dryRun {
		fmt.Fprintln(os.Stderr, "error: --write and --dry-run cannot be used together")
		os.Exit(1)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	rpt := report.New(*verbose, os.Stdout, os.Stderr)

	rpt.VerboseLog("loading packages: %s", *input)

	pkgs, err := loader.LoadPackages(*input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(pkgs) == 0 {
		fmt.Fprintln(os.Stderr, "error: no Go packages found")
		os.Exit(1)
	}

	hasChi := false
	var chiPkgName, chiImpPath string
	for _, pkg := range pkgs {
		if loader.HasChiRouter(pkg) {
			hasChi = true
			chiPkgName = loader.GetChiPackageName(pkg)
			chiImpPath = loader.FindChiImportPath(pkg)
			break
		}
	}

	if !hasChi {
		fmt.Fprintln(os.Stderr, "error: unsupported router: only chi is supported")
		os.Exit(1)
	}

	acfg := &analyzer.AnalysisConfig{
		InferRequestBody:  cfg.Rules.InferRequestBody,
		InferResponseBody: cfg.Rules.InferResponseBody,
		InferQueryParams:  cfg.Rules.InferQueryParams,
		InferPathParams:   cfg.Rules.InferPathParams,
		InferStatusCodes:  cfg.Rules.InferStatusCodes,
		InferAuth:         cfg.Rules.InferAuth,
		BasePath:          cfg.BasePath,
		DefaultErrorType:  cfg.DefaultErrorType,
		AuthHeader:        cfg.Auth.JWT.Header,
		AuthPrefix:        cfg.Auth.JWT.Prefix,
		SecurityName:      cfg.Auth.JWT.SecurityName,
	}

	rf := analyzer.NewRouteFinder(acfg, chiPkgName, chiImpPath, rpt.VerboseLog)
	ba := analyzer.NewBodyAnalyzer(acfg, chiPkgName, rpt.VerboseLog)
	aa := analyzer.NewAuthAnalyzer(acfg, chiPkgName, rpt.VerboseLog)
	sg := generator.NewSwaggerGen(acfg)
	rw := rewriter.New(*dryRun, *verbose, rpt.VerboseLog)

	allRoutes := []*analyzer.RouteInfo{}
	for _, pkg := range pkgs {
		if !loader.HasChiRouter(pkg) {
			rpt.VerboseLog("package %s: no chi router found, skipping", pkg.PkgPath)
			continue
		}
		routes := rf.FindRoutes(pkg, pkgs)
		allRoutes = append(allRoutes, routes...)
	}

	rpt.VerboseLog("found %d routes total", len(allRoutes))

	commentMap := make(map[*analyzer.RouteInfo]string)
	for _, route := range allRoutes {
		if route.Handler == nil {
			rpt.VerboseLog("skip: %s %s -> no handler found", route.Method, route.Path)
			continue
		}

		// Check for existing swagger
		if route.Handler.FuncDecl != nil && route.Handler.FuncDecl.Doc != nil {
			if generator.HasExistingSwagger(route.Handler.FuncDecl.Doc) {
				handlerName := route.Handler.Name
				if handlerName == "" {
					handlerName = route.Method + " " + route.Path
				}
				rpt.AddSkipped(route.Method, route.Path, handlerName, "existing swagger found")
				if *failOnExisting {
					fmt.Fprintf(os.Stderr, "error: existing swagger documentation found for %s\n", handlerName)
					os.Exit(1)
				}
				rpt.VerboseLog("skip: existing swagger documentation found for %s", handlerName)
				continue
			}
		}

		bodyAnalysis := ba.AnalyzeHandler(route.Handler)
		authResult := aa.AnalyzeAuth(route)

		comment := sg.GenerateComment(route, bodyAnalysis, authResult)
		commentMap[route] = comment

		handlerName := route.Handler.Name
		if handlerName == "" {
			handlerName = route.Method + " " + route.Path
		}

		rpt.AddGenerated(route.Method, route.Path, handlerName)
		rpt.VerboseLog("generated swagger for %s %s -> %s", route.Method, route.Path, handlerName)

		if bodyAnalysis.RequestBody != nil {
			rpt.VerboseLog("found request body: %s", bodyAnalysis.RequestBody.TypeName)
		}
		if bodyAnalysis.ResponseBody != nil {
			rpt.VerboseLog("found success response: %d %s", bodyAnalysis.ResponseBody.StatusCode, bodyAnalysis.ResponseBody.TypeName)
		}
		for _, er := range bodyAnalysis.ErrorResponses {
			rpt.VerboseLog("found failure response: %d %s", er.StatusCode, er.TypeName)
		}
		if authResult.IsProtected {
			rpt.VerboseLog("found auth middleware: %s", authResult.MiddlewareName)
		}
	}

	if *dryRun {
		rpt.PrintDryRun()
		rpt.PrintWarnings()
		return
	}

	if *write {
		if err := rw.InsertComments(allRoutes, commentMap); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("successfully added swagger comments to %d handlers\n", len(commentMap))
		return
	}

	// Default: just print report
	rpt.PrintDryRun()
	rpt.PrintWarnings()
	fmt.Fprintln(os.Stderr, "\nuse --write to apply changes or --dry-run to preview")
}
