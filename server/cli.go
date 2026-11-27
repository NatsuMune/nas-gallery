package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

const defaultConfigPath = "config.yaml"

// bcryptCost per the security spec.
const bcryptCost = 10

// cmdInit writes an example config if absent and prepares dataDir (DB, secret,
// thumb cache). Safe to run repeatedly.
func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	cfgPath := fs.String("c", defaultConfigPath, "path of the config file to create/use")
	listenAddr := fs.String("listen", "", "listenAddr to write when creating a new config")
	serverName := fs.String("server-name", "", "serverName to write when creating a new config")
	mediaPaths := fs.String("media", "", "comma-separated mediaPaths to write when creating a new config")
	dataDir := fs.String("data-dir", "", "dataDir to write when creating a new config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	created, err := writeInitialConfig(*cfgPath, *listenAddr, *serverName, *mediaPaths, *dataDir)
	if err != nil {
		return err
	}
	cfg, err := LoadConfig(*cfgPath)
	if err != nil {
		return fmt.Errorf("config %s is invalid: %w", *cfgPath, err)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return fmt.Errorf("create dataDir: %w", err)
	}
	if err := os.MkdirAll(cfg.thumbsDir(), 0o700); err != nil {
		return fmt.Errorf("create thumb cache dir: %w", err)
	}
	store, err := OpenStore(cfg.dbPath())
	if err != nil {
		return err
	}
	defer store.Close()
	if _, err := store.EnsureSecret(); err != nil {
		return err
	}
	if created {
		fmt.Printf("wrote example config to %s\n", *cfgPath)
	} else {
		fmt.Printf("config already present at %s (left untouched)\n", *cfgPath)
	}
	fmt.Printf("database ready at %s\n", cfg.dbPath())
	if len(cfg.MediaPaths) == 0 {
		fmt.Println("note: mediaPaths is empty — edit the config and point it at your media roots")
	}
	fmt.Println("next: nas-gallery user add <name> && nas-gallery serve")
	return nil
}

// cmdUser implements "user add|list|rm".
func cmdUser(args []string) error {
	if len(args) == 0 {
		return errors.New(`usage: nas-gallery user add <name> | user list | user rm <name>`)
	}
	sub, rest := args[0], args[1:]
	fs := flag.NewFlagSet("user "+sub, flag.ContinueOnError)
	cfgPath := fs.String("c", defaultConfigPath, "path of the config file")
	pwEnv := fs.String("password-env", "", "environment variable holding password (for non-interactive setup)")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	positional := fs.Args()

	cfg, err := LoadConfig(*cfgPath)
	if err != nil {
		return fmt.Errorf("cannot load config %s — run \"nas-gallery init\" first: %w", *cfgPath, err)
	}
	if _, err := os.Stat(cfg.dbPath()); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("database %s not found — run \"nas-gallery init\" first", cfg.dbPath())
	}
	store, err := OpenStore(cfg.dbPath())
	if err != nil {
		return err
	}
	defer store.Close()

	switch sub {
	case "add":
		if len(positional) != 1 {
			return errors.New("usage: nas-gallery user add <name>")
		}
		return userAdd(store, positional[0], *pwEnv)
	case "list":
		if len(positional) != 0 {
			return errors.New("usage: nas-gallery user list")
		}
		return userList(store)
	case "rm":
		if len(positional) != 1 {
			return errors.New("usage: nas-gallery user rm <name>")
		}
		return userRm(store, positional[0])
	default:
		return fmt.Errorf("unknown user subcommand %q (add, list, rm)", sub)
	}
}

func userAdd(store *Store, name, pwEnv string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("username must not be empty")
	}
	var pw string
	if pwEnv != "" {
		var ok bool
		pw, ok = os.LookupEnv(pwEnv)
		if !ok {
			return fmt.Errorf("environment variable %q is not set", pwEnv)
		}
		if len(pw) < 4 {
			return errors.New("password too short: minimum 4 characters")
		}
	} else {
		var err error
		pw, err = readPassword("Password (min 4 chars): ")
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		if len(pw) < 4 {
			return errors.New("password too short: minimum 4 characters")
		}
		confirm, err := readPassword("Confirm password: ")
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		if pw != confirm {
			return errors.New("passwords do not match")
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	id, err := store.CreateUser(name, string(hash))
	if err != nil {
		return err
	}
	fmt.Printf("user %q created (id %d)\n", name, id)
	return nil
}

func writeInitialConfig(path, listenAddr, serverName, mediaCSV, dataDir string) (bool, error) {
	if listenAddr == "" && serverName == "" && mediaCSV == "" && dataDir == "" {
		return writeExampleConfig(path)
	}
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stat config: %w", err)
	}
	cfg := defaultConfig()
	if strings.TrimSpace(listenAddr) != "" {
		cfg.ListenAddr = strings.TrimSpace(listenAddr)
	}
	if strings.TrimSpace(serverName) != "" {
		cfg.ServerName = strings.TrimSpace(serverName)
	}
	if strings.TrimSpace(dataDir) != "" {
		cfg.DataDir = strings.TrimSpace(dataDir)
	}
	if strings.TrimSpace(mediaCSV) != "" {
		for _, part := range strings.Split(mediaCSV, ",") {
			if p := strings.TrimSpace(part); p != "" {
				cfg.MediaPaths = append(cfg.MediaPaths, p)
			}
		}
	}
	if err := cfg.validate(); err != nil {
		return false, err
	}
	body, err := yaml.Marshal(&cfg)
	if err != nil {
		return false, fmt.Errorf("marshal config: %w", err)
	}
	if err := os.MkdirAll(absOrSelf(path), 0o755); err != nil {
		return false, fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return false, fmt.Errorf("write config: %w", err)
	}
	return true, nil
}

func userList(store *Store) error {
	users, err := store.ListUsers()
	if err != nil {
		return err
	}
	if len(users) == 0 {
		fmt.Println("no users — add one with \"nas-gallery user add <name>\"")
		return nil
	}
	fmt.Printf("%-5s %-20s %s\n", "ID", "USERNAME", "CREATED")
	for _, u := range users {
		fmt.Printf("%-5d %-20s %s\n", u.ID, u.Username, u.CreatedAt.Format(time.RFC3339))
	}
	return nil
}

func userRm(store *Store, name string) error {
	u, err := store.UserByUsername(name)
	if err != nil {
		return err
	}
	if u == nil {
		return fmt.Errorf("no such user: %q", name)
	}
	if err := store.DeleteUser(u.ID); err != nil {
		return err
	}
	fmt.Printf("user %q removed (tokens and favorites revoked)\n", name)
	return nil
}

// cmdServe runs the HTTP server until SIGINT/SIGTERM.
func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	cfgPath := fs.String("c", defaultConfigPath, "path of the config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := LoadConfig(*cfgPath)
	if err != nil {
		return fmt.Errorf("cannot load config %s — run \"nas-gallery init\" first: %w", *cfgPath, err)
	}
	if err := os.MkdirAll(cfg.thumbsDir(), 0o700); err != nil {
		return fmt.Errorf("create thumb cache dir: %w", err)
	}
	store, err := OpenStore(cfg.dbPath())
	if err != nil {
		return err
	}
	defer store.Close()
	secret, err := store.EnsureSecret()
	if err != nil {
		return err
	}
	scanner := NewScanner(cfg, store, cfg.thumbsDir())
	app := NewApp(cfg, store, secret, scanner)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go scanner.Run(ctx) // async initial scan + periodic rescans

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           app.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("nas-gallery %s listening on %s (server name %q, %d media root(s), dataDir %s)",
			Version, cfg.ListenAddr, cfg.ServerName, len(cfg.MediaPaths), cfg.DataDir)
		errCh <- srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server error: %w", err)
		}
	case <-ctx.Done():
		log.Println("shutting down…")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown failed: %w", err)
		}
		log.Println("bye")
	}
	return nil
}
