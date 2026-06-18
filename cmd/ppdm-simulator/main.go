package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"ppdm-simul/internal/auth"
	"ppdm-simul/internal/loader"
	"ppdm-simul/internal/server"
	"ppdm-simul/internal/tlsutil"
)

func main() {
	openapiDir := flag.String("openapi-dir", "openapi-json", "Directory containing PPDM OpenAPI JSON files")
	host := flag.String("host", "0.0.0.0", "Bind host")
	port := flag.Int("port", 8443, "Bind port")
	certFile := flag.String("cert", "", "TLS certificate file (PEM). Defaults to ./ssl/cert.pem")
	keyFile := flag.String("key", "", "TLS private key file (PEM). Defaults to ./ssl/key.pem")
	sslDir := flag.String("ssl-dir", "ssl", "Directory for stored TLS certificate and key")
	noAuth := flag.Bool("no-auth", false, "Disable bearer token checks")
	quiet := flag.Bool("quiet", false, "Disable API request/response logging")
	flag.Parse()

	absDir, err := filepath.Abs(*openapiDir)
	if err != nil {
		log.Fatalf("resolve openapi dir: %v", err)
	}
	info, err := os.Stat(absDir)
	if err != nil || !info.IsDir() {
		log.Fatalf("OpenAPI directory not found: %s", absDir)
	}

	bundle, err := loader.LoadDirectory(absDir)
	if err != nil {
		log.Fatalf("load OpenAPI specs: %v", err)
	}

	certManager, err := tlsutil.NewManager(*certFile, *keyFile, *sslDir, *host)
	if err != nil {
		log.Fatalf("load TLS certificate: %v", err)
	}
	resolved := certManager.Resolved()

	authManager := auth.NewManager(!*noAuth, 3600)
	srv := server.New(bundle, authManager, !*quiet)

	addr := fmt.Sprintf("%s:%d", *host, *port)
	httpServer := &http.Server{
		Addr:      addr,
		Handler:   srv.Handler(),
		TLSConfig: certManager.TLSConfig(),
	}

	listener, err := tls.Listen("tcp", addr, httpServer.TLSConfig)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	renewCtx, stopRenew := context.WithCancel(context.Background())
	defer stopRenew()
	certManager.StartAutoRenew(renewCtx)

	go func() {
		log.Printf("PPDM simulator listening on https://%s (%d operations)", displayAddr(*host, *port), len(bundle.Operations))
		switch {
		case *certFile != "" || *keyFile != "":
			log.Printf("using TLS certificate from %s and %s", resolved.CertPath, resolved.KeyPath)
		case resolved.Managed:
			log.Printf("using managed self-signed TLS certificate from %s (7-day lifetime, auto-renewal enabled)", *sslDir)
		default:
			log.Printf("using stored TLS certificate from %s", *sslDir)
		}
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("shutting down...")
	stopRenew()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
	log.Println("server stopped")
}

func displayAddr(host string, port int) string {
	if host == "0.0.0.0" || host == "::" {
		return fmt.Sprintf("localhost:%d", port)
	}
	return net.JoinHostPort(host, fmt.Sprintf("%d", port))
}
