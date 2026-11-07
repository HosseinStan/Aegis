package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"company.local/aegis/apps/aegis-api/internal/connectors/cisco_ios_ssh"
	"company.local/aegis/apps/aegis-api/internal/credentials"
	"company.local/aegis/apps/aegis-api/internal/repositories/sqlite"
	"golang.org/x/crypto/ssh"
)

const maxConfigBytes = 2 << 20

func main() {
	if err := run(); err != nil {
		log.Fatal("Lab Manager: ", err)
	}
}

func run() error {
	dataDir := flag.String("data-dir", "", "Dedicated private Lab data directory outside Git")
	uiDir := flag.String("ui-dir", "", "Directory containing the built Vue Lab UI")
	approvals := flag.String("approvals", "", "Local file with up to two exact target/host-key pins")
	enableSSH := flag.Bool("enable-ssh", false, "Explicitly enable read-only SSH to pinned Lab targets")
	port := flag.Int("port", 0, "Loopback HTTP port (0 selects a free port)")
	flag.Parse()
	if *dataDir == "" || *uiDir == "" || *port < 0 || *port > 65535 || *enableSSH != (*approvals != "") {
		return errors.New("provide -data-dir and -ui-dir; SSH requires both -enable-ssh and -approvals")
	}
	operator := os.Getenv("AEGIS_LAB_OPERATOR_PASSWORD")
	viewer := os.Getenv("AEGIS_LAB_VIEWER_PASSWORD")
	_ = os.Unsetenv("AEGIS_LAB_OPERATOR_PASSWORD")
	_ = os.Unsetenv("AEGIS_LAB_VIEWER_PASSWORD")
	login, err := labLogin(operator, viewer)
	if err != nil {
		return err
	}
	pins, err := loadPins(*approvals)
	if err != nil {
		return err
	}
	connector, err := cisco_ios_ssh.NewPinned(pins, 3*time.Second, 5*time.Second, 12*time.Second, maxConfigBytes)
	if err != nil {
		return err
	}

	ui, err := os.OpenRoot(*uiDir)
	if err != nil {
		return errors.New("open the built Lab UI directory")
	}
	defer ui.Close()
	if info, err := ui.Stat("index.html"); err != nil || !info.Mode().IsRegular() {
		return errors.New("build the Lab Vue UI before starting")
	}
	storage, err := openLabStorage(*dataDir)
	if err != nil {
		return err
	}
	defer storage.lock.Close()
	defer clear(storage.key)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store, err := sqlite.Open(ctx, storage.database)
	if err != nil {
		return errors.New("could not open the dedicated Lab SQLite database")
	}
	defer store.Close()
	vault, err := credentials.NewStore(store, storage.key)
	if err != nil {
		return err
	}
	// Detect accidental loss or replacement of the Lab credential key before serving.
	if err := checkVault(ctx, vault); err != nil {
		return err
	}
	manager, err := newManager(store, vault, connector, storage.backups)
	if err != nil {
		return err
	}
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: *port})
	if err != nil {
		return err
	}
	defer listener.Close()
	host := listener.Addr().String()
	site := newLabSite(host, login, manager, ui.FS())
	server := &http.Server{
		Handler: site.routes(), ReadHeaderTimeout: 4 * time.Second, ReadTimeout: 8 * time.Second,
		WriteTimeout: 28 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10,
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	fmt.Printf("AEGIS LAB — http://%s/\nApproved SSH destinations: %d. Backup creation locked.\n", host, len(pins))
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			return err
		}
	}
	return nil
}

func loadPins(path string) (map[netip.AddrPort]ssh.PublicKey, error) {
	pins := make(map[netip.AddrPort]ssh.PublicKey)
	if path == "" {
		return pins, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("could not open the local Lab approvals file")
	}
	defer file.Close()
	var input struct {
		Targets []struct {
			Address string `json:"address"`
			HostKey string `json:"host_key"`
		} `json:"targets"`
	}
	const maxApprovalBytes = 32 << 10
	raw, err := io.ReadAll(io.LimitReader(file, maxApprovalBytes+1))
	if err != nil || len(raw) > maxApprovalBytes {
		return nil, errors.New("could not read the Lab approvals file within its size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || decoder.Decode(new(any)) != io.EOF || len(input.Targets) < 1 || len(input.Targets) > 2 {
		return nil, errors.New("Lab approvals must contain one or two target/host-key entries")
	}
	for _, item := range input.Targets {
		target, err := netip.ParseAddrPort(item.Address)
		if err != nil || target.Addr().Is4In6() || target.Addr().Zone() != "" || target.Port() == 0 || pins[target] != nil {
			return nil, errors.New("Lab approvals require unique literal IP:port targets")
		}
		key, _, options, rest, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(item.HostKey)))
		if err != nil || len(options) != 0 || len(strings.TrimSpace(string(rest))) != 0 || strings.ContainsAny(strings.TrimSpace(item.HostKey), "\r\n") {
			return nil, errors.New("each Lab approval requires one independently verified SSH public key")
		}
		if _, certificate := key.(*ssh.Certificate); certificate {
			return nil, errors.New("Lab pins require plain SSH host public keys")
		}
		pins[target] = key
	}
	return pins, nil
}

func randomBytes(size int) []byte {
	bytes := make([]byte, size)
	rand.Read(bytes)
	return bytes
}
