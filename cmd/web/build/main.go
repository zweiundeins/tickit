package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/mbolli/tickit/config"
	"github.com/mbolli/tickit/web/ui"

	"github.com/bep/godartsass/v2"
	"github.com/evanw/esbuild/pkg/api"
)

func main() {
	watch := flag.Bool("watch", false, "Enable watcher mode")
	flag.Parse()

	if err := run(*watch); err != nil {
		slog.Error("failure", "error", err)
		os.Exit(1)
	}
}

func run(watch bool) error {
	opts := api.BuildOptions{
		EntryPoints:       []string{ui.ScriptDirectoryPath + "/index.ts"},
		Bundle:            true,
		Format:            api.FormatESModule,
		LogLevel:          api.LogLevelInfo,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
		MinifyWhitespace:  true,
		Outdir:            ui.StaticDirectoryPath,
		OutExtension:      map[string]string{".js": ".mjs"},
		Plugins: []api.Plugin{{
			Name: "hotreload",
			Setup: func(build api.PluginBuild) {
				build.OnEnd(func(result *api.BuildResult) (api.OnEndResult, error) {
					slog.Info("build complete", "errors", len(result.Errors), "warnings", len(result.Warnings))
					if watch && len(result.Errors) == 0 {
						http.Get(fmt.Sprintf("http://%s:%s/hotreload", config.Global.Host, config.Global.HttpsPort))
					}
					return api.OnEndResult{}, nil
				})
			},
		}},
		Sourcemap: api.SourceMapLinked,
		Target:    api.ES2022,
		Write:     true,
		Tsconfig:  ui.StaticDirectoryPath + "/../tsconfig.json",
		Stdin: &api.StdinOptions{
			Contents:   compileSass(ui.StylesDirectoryPath + "/styles.scss"),
			ResolveDir: ui.StylesDirectoryPath,
			Sourcefile: "index.css",
			Loader:     api.LoaderCSS,
		},
	}

	if watch {
		slog.Info("Watching...")
		ctx, err := api.Context(opts)
		if err != nil {
			return err
		}

		if err := ctx.Watch(api.WatchOptions{}); err != nil {
			return err
		}

		<-make(chan struct{})
		return nil
	}

	slog.Info("Building...")

	result := api.Build(opts)

	if len(result.Errors) > 0 {
		errs := make([]error, len(result.Errors))
		for i, err := range result.Errors {
			errs[i] = errors.New(err.Text)
		}
		return errors.Join(errs...)
	}

	return nil
}

func compileSass(inputFile string) string {
	// Create a new Sass compiler
	comp, err := godartsass.Start(godartsass.Options{
		DartSassEmbeddedFilename: "dart-sass",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer comp.Close()

	abs, err := filepath.Abs(inputFile)
	if err != nil {
		log.Fatalf("failed to get absolute path: %v", err)
	}
	if _, err := os.Stat(abs); err != nil {
		log.Fatalf("sass input file not found or unreadable: %v", err)
	}
	res, err := comp.Execute(godartsass.Args{
		URL:             abs,
		EnableSourceMap: true,
	})
	if err != nil {
		log.Fatalf("sass error: %v (file: %s, operation: %s)", err, abs, "compile")
	}

	return res.CSS
}
