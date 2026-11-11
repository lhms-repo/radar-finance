// Command radar runs the Radar web service and its maintenance commands.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	// Embedded time zone database, so the minimal image knows America/Sao_Paulo.
	_ "time/tzdata"

	"golang.org/x/term"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/backup"
	"github.com/italogfx/radar-finance/internal/config"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/server"
	"github.com/italogfx/radar-finance/web"
)

// version is set at build time with -ldflags.
var version = "dev"

const usage = `Uso: radar <comando>

Comandos:
  serve                  inicia o servidor (padrão)
  user add               cria um usuário e a pessoa ligada a ele
  user password          troca a senha de um usuário
  user list              lista os usuários
  backup                 gera um backup manual
  healthcheck            verifica se o servidor responde
  version                mostra a versão
`

// main dispatches the subcommand and exits with its status code.
func main() {
	os.Exit(run(os.Args[1:]))
}

// run executes the subcommand named in args and returns the exit code.
func run(args []string) int {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cmd := "serve"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve(log)
	case "user":
		err = userCommand(args, log)
	case "backup":
		err = backupCommand(log)
	case "healthcheck":
		err = healthcheck()
	case "version":
		fmt.Println(version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		return 1
	}
	return 0
}

// serve opens the database, starts the background jobs and serves HTTP until a signal.
func serve(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	conn, err := db.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := db.Migrate(ctx, conn, log); err != nil {
		return err
	}
	accounts, err := account.NewService(conn)
	if err != nil {
		return err
	}
	sessions := auth.NewSessionManager(conn, auth.SessionOptions{
		Idle: cfg.SessionIdle, Lifetime: cfg.SessionLifetime, Secure: cfg.CookieSecure,
	})
	backups := backup.New(conn, backup.Options{
		Dir: cfg.BackupDir, Keep: cfg.BackupKeep, Hour: cfg.BackupHour, Loc: cfg.Loc,
	}, log)

	go backups.Start(ctx)
	go auth.CleanupSessions(ctx, conn, time.Hour, log)

	handler := server.New(server.Deps{
		Config:    cfg,
		DB:        conn,
		Accounts:  accounts,
		Sessions:  sessions,
		Backups:   backups,
		Throttle:  auth.NewThrottle(5, 15*time.Minute, 5*time.Minute, nil),
		Version:   version,
		StartedAt: time.Now(),
		Log:       log,
		Assets:    web.Assets(),
	})
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("radar listening", "addr", cfg.Addr, "version", version, "db", cfg.DBPath)
		errCh <- srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// openForCommand opens and migrates the database for a maintenance command.
func openForCommand(log *slog.Logger) (*account.Service, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	conn, err := db.Open(cfg.DBPath)
	if err != nil {
		return nil, nil, err
	}
	if err := db.Migrate(context.Background(), conn, log); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	accounts, err := account.NewService(conn)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	return accounts, func() { _ = conn.Close() }, nil
}

// userCommand handles the "user add", "user password" and "user list" subcommands.
func userCommand(args []string, log *slog.Logger) error {
	if len(args) == 0 {
		return errors.New("use: radar user add|password|list")
	}
	sub, args := args[0], args[1:]
	fs := flag.NewFlagSet("user "+sub, flag.ContinueOnError)
	login := fs.String("login", "", "login do usuário")
	name := fs.String("name", "", "nome da pessoa")
	fromStdin := fs.Bool("password-stdin", false, "lê a senha da primeira linha da entrada padrão")
	if err := fs.Parse(args); err != nil {
		return err
	}
	accounts, closeDB, err := openForCommand(log)
	if err != nil {
		return err
	}
	defer closeDB()
	ctx := context.Background()

	switch sub {
	case "add":
		password, err := readPassword(*fromStdin)
		if err != nil {
			return err
		}
		u, err := accounts.Create(ctx, *login, *name, password)
		if err != nil {
			return err
		}
		fmt.Printf("Usuário %q criado para %s.\n", u.Login, u.Name)
	case "password":
		if *login == "" {
			return errors.New("informe --login")
		}
		password, err := readPassword(*fromStdin)
		if err != nil {
			return err
		}
		if err := accounts.SetPassword(ctx, *login, password); err != nil {
			return err
		}
		fmt.Println("Senha alterada.")
	case "list":
		users, err := accounts.List(ctx)
		if err != nil {
			return err
		}
		for _, u := range users {
			fmt.Printf("%d\t%s\t%s\n", u.ID, u.Login, u.PersonName)
		}
	default:
		return fmt.Errorf("subcomando desconhecido: user %s", sub)
	}
	return nil
}

// readPassword reads a password from stdin, asking twice without echo on a terminal.
func readPassword(fromStdin bool) (string, error) {
	fd := int(os.Stdin.Fd())
	if fromStdin || !term.IsTerminal(fd) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	fmt.Print("Senha: ")
	first, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return "", err
	}
	fmt.Print("Repita a senha: ")
	second, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return "", err
	}
	if string(first) != string(second) {
		return "", errors.New("as senhas não conferem")
	}
	return string(first), nil
}

// backupCommand runs a manual backup against the configured database.
func backupCommand(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	conn, err := db.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer conn.Close()
	svc := backup.New(conn, backup.Options{Dir: cfg.BackupDir, Keep: cfg.BackupKeep, Hour: cfg.BackupHour, Loc: cfg.Loc}, log)
	info, err := svc.Run(context.Background(), backup.KindManual)
	if err != nil {
		return err
	}
	fmt.Printf("Backup criado: %s (%d bytes)\n", info.Name, info.SizeBytes)
	return nil
}

// healthcheck calls the local ping endpoint, for the container health check.
func healthcheck() error {
	addr := os.Getenv("RADAR_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/api/ping")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
