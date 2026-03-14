package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"slpp/internal/auth"
	"slpp/internal/bench"
	"slpp/internal/control"
	"slpp/internal/core"
	"slpp/internal/transport"
	"slpp/pkg/api"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "run", "service":
		runServer(os.Args[2:])
	case "check":
		checkServer(os.Args[2:])
	case "gen-token":
		genToken(os.Args[2:])
	case "revoke-token":
		revokeToken(os.Args[2:])
	case "stats":
		printStats(os.Args[2:])
	case "bench":
		runBench(os.Args[2:])
	case "version":
		fmt.Println(version)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: slppd {run|service|check|gen-token|revoke-token|stats|bench|version}\n")
}

func runServer(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	listen := fs.String("listen", ":8443", "listen address")
	certFile := fs.String("cert", "", "tls certificate file")
	keyFile := fs.String("key", "", "tls key file")
	tokenFile := fs.String("token-file", "tokens.json", "token store file")
	controlSock := fs.String("control-socket", "/tmp/slppd.sock", "unix socket for control API")
	jsonOut := fs.Bool("json", false, "json output")
	fs.Parse(args)

	stats := core.NewStats()
	store, err := auth.NewStore(*tokenFile)
	if err != nil {
		exitErr(err, *jsonOut)
	}
	handler := &transport.TunnelHandler{Validator: store, Stats: stats}
	server := transport.NewHTTPServer(core.ServerConfig{
		ListenAddr:    *listen,
		CertFile:      *certFile,
		KeyFile:       *keyFile,
		ControlSocket: *controlSock,
	}, handler)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	controlServer := control.NewServer(*controlSock, stats)
	go func() {
		if err := controlServer.Run(ctx); err != nil && ctx.Err() == nil {
			slog.Error("control server stopped", "err", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if *jsonOut {
		_ = json.NewEncoder(os.Stdout).Encode(api.OutputEnvelope{OK: true, Message: "server starting", Data: map[string]string{"listen": *listen}})
	} else {
		fmt.Printf("slppd listening on %s\n", *listen)
	}
	if *certFile == "" || *keyFile == "" {
		exitErr(fmt.Errorf("both --cert and --key are required"), *jsonOut)
	}
	if err := server.ListenAndServeTLS(*certFile, *keyFile); err != nil && ctx.Err() == nil {
		exitErr(err, *jsonOut)
	}
}

func checkServer(args []string) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	listen := fs.String("listen", ":8443", "listen address")
	tokenFile := fs.String("token-file", "tokens.json", "token store file")
	jsonOut := fs.Bool("json", false, "json output")
	fs.Parse(args)
	_, err := auth.NewStore(*tokenFile)
	if err != nil {
		exitErr(err, *jsonOut)
	}
	out := api.OutputEnvelope{OK: true, Message: "configuration valid", Data: map[string]string{"listen": *listen, "token_file": *tokenFile}}
	printEnvelope(out, *jsonOut)
}

func genToken(args []string) {
	fs := flag.NewFlagSet("gen-token", flag.ExitOnError)
	tokenFile := fs.String("token-file", "tokens.json", "token store file")
	ttl := fs.Duration("ttl", 24*time.Hour, "token ttl")
	jsonOut := fs.Bool("json", false, "json output")
	fs.Parse(args)
	store, err := auth.NewStore(*tokenFile)
	if err != nil {
		exitErr(err, *jsonOut)
	}
	rec, err := store.Generate(*ttl)
	if err != nil {
		exitErr(err, *jsonOut)
	}
	printEnvelope(api.OutputEnvelope{OK: true, Data: rec}, *jsonOut)
}

func revokeToken(args []string) {
	fs := flag.NewFlagSet("revoke-token", flag.ExitOnError)
	tokenFile := fs.String("token-file", "tokens.json", "token store file")
	id := fs.String("id", "", "token id")
	jsonOut := fs.Bool("json", false, "json output")
	fs.Parse(args)
	if *id == "" {
		exitErr(fmt.Errorf("--id is required"), *jsonOut)
	}
	store, err := auth.NewStore(*tokenFile)
	if err != nil {
		exitErr(err, *jsonOut)
	}
	if err := store.Revoke(*id); err != nil {
		exitErr(err, *jsonOut)
	}
	printEnvelope(api.OutputEnvelope{OK: true, Message: "token revoked"}, *jsonOut)
}

func printStats(args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	socket := fs.String("control-socket", "/tmp/slppd.sock", "unix socket for control API")
	jsonOut := fs.Bool("json", false, "json output")
	fs.Parse(args)
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", *socket)
			},
		},
	}
	resp, err := client.Get("http://unix/stats")
	if err != nil {
		stats := core.NewStats().Snapshot()
		printEnvelope(api.OutputEnvelope{OK: true, Message: "control socket unavailable, showing empty snapshot", Data: stats}, *jsonOut)
		return
	}
	defer resp.Body.Close()
	var out api.OutputEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		exitErr(err, *jsonOut)
	}
	printEnvelope(out, *jsonOut)
}

func runBench(args []string) {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	iters := fs.Int("n", 1000, "iterations")
	jsonOut := fs.Bool("json", false, "json output")
	fs.Parse(args)
	result := bench.CodecEncode(*iters)
	printEnvelope(api.OutputEnvelope{OK: true, Data: result}, *jsonOut)
}

func printEnvelope(out api.OutputEnvelope, asJSON bool) {
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(out)
		return
	}
	if out.Message != "" {
		fmt.Println(out.Message)
	}
	if out.Data != nil {
		data, _ := json.MarshalIndent(out.Data, "", "  ")
		fmt.Println(string(data))
	}
}

func exitErr(err error, asJSON bool) {
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(api.OutputEnvelope{OK: false, Message: err.Error()})
	} else {
		fmt.Fprintln(os.Stderr, "error:", err)
	}
	os.Exit(1)
}
