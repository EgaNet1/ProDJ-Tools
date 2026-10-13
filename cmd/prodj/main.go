// Command prodj starts ProDJ Tools: a local server holding the analysis
// engine, and a window showing the interface.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"

	"prodjtools/internal/desktop"
	"prodjtools/internal/i18n"
	"prodjtools/internal/server"
)

// version is set at build time: -ldflags "-X main.version=1.2.3".
var version = "dev"

func main() {
	var (
		port     = flag.Int("port", 0, "server port (0: any free port)")
		headless = flag.Bool("no-window", false, "do not open a window; run until the process is stopped")
		dir      = flag.String("dir", "", "open this folder at start")
	)
	flag.Parse()

	if err := run(*port, *headless, *dir); err != nil {
		log.Print(err)
		desktop.Alert("ProDJ Tools", err.Error())
		os.Exit(1)
	}
}

func run(port int, headless bool, dir string) error {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	dataDir := filepath.Join(cacheDir, "ProDJTools")
	app, err := server.New(dataDir, version)
	if err != nil {
		return err
	}
	defer app.Shutdown()

	// The release build has no console, so the log goes to a file.
	if logFile, err := os.Create(filepath.Join(dataDir, "prodj.log")); err == nil {
		defer logFile.Close()
		log.SetOutput(logFile)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("error.server"), err)
	}
	url := "http://" + listener.Addr().String()
	go http.Serve(listener, app.Handler())
	log.Printf("listening on %s", url)

	if dir != "" {
		if err := app.Open(dir); err != nil {
			log.Print(err)
		}
	}

	if headless {
		fmt.Println("ProDJ Tools:", url)
		interrupted := make(chan os.Signal, 1)
		signal.Notify(interrupted, os.Interrupt)
		<-interrupted
		return nil
	}

	if err := desktop.OpenWindow(url, filepath.Join(dataDir, "window")); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("error.window"), err)
	}
	app.WatchWindow()
	<-app.Closed()
	return nil
}
