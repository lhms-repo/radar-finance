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

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/agenda"
	"github.com/italogfx/radar-finance/internal/alelo"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/backup"
	"github.com/italogfx/radar-finance/internal/bill"
	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/category"
	"github.com/italogfx/radar-finance/internal/config"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/events"
	"github.com/italogfx/radar-finance/internal/financing"
	"github.com/italogfx/radar-finance/internal/importer"
	"github.com/italogfx/radar-finance/internal/income"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/market"
	"github.com/italogfx/radar-finance/internal/nfce"
	"github.com/italogfx/radar-finance/internal/notify"
	"github.com/italogfx/radar-finance/internal/receivable"
	"github.com/italogfx/radar-finance/internal/report"
	"github.com/italogfx/radar-finance/internal/savings"
	"github.com/italogfx/radar-finance/internal/server"
	"github.com/italogfx/radar-finance/web"
)

// version is set at build time with -ldflags.
var version = "dev"

const usage = `Uso: radar <comando>

Comandos:
  serve                  inicia o servidor (padrão)
  user add               cria uma pessoa e o usuário dela (--login, --name; --group e --admin para já pôr num grupo)
  user pin               define o PIN de um usuário (--login; o PIN é lido da entrada)
  user list              lista os usuários
  group create           cria um grupo com o primeiro administrador (--name, --admin LOGIN)
  group add              põe um usuário num grupo (--group NOME, --login; --as-admin para administrador)
  group list             lista os grupos e os membros
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
	case "group":
		err = groupCommand(args, log)
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
	accounts := account.NewService(conn)
	sessions := auth.NewSessionManager(conn, auth.SessionOptions{
		Idle: cfg.SessionIdle, Lifetime: cfg.SessionLifetime, Secure: cfg.CookieSecure,
	})
	backups := backup.New(conn, backup.Options{
		Dir: cfg.BackupDir, Keep: cfg.BackupKeep, Hour: cfg.BackupHour, Loc: cfg.Loc,
	}, log)

	go backups.Start(ctx)
	go auth.CleanupSessions(ctx, conn, time.Hour, log)

	cards, bills, financings := card.NewService(conn), bill.NewService(conn), financing.NewService(conn)
	sav, reports := savings.NewService(conn), report.NewService(conn, cards)
	markets := market.NewService(conn)
	if cfg.ReceiptFixtures != "" {
		log.Warn("receipts read from fixtures, not from the Sefaz", "dir", cfg.ReceiptFixtures)
		markets.WithFetcher(nfce.FixtureClient{Dir: cfg.ReceiptFixtures})
	}
	// The live updates hub is closed when shutdown starts, so open streams do not hold it (RF-71).
	hub := events.NewHub()
	handler := server.New(server.Deps{
		Config:        cfg,
		DB:            conn,
		Accounts:      accounts,
		Categories:    category.NewService(conn),
		Ledger:        ledger.NewService(conn),
		Incomes:       income.NewService(conn),
		Cards:         cards,
		Bills:         bills,
		Financings:    financings,
		Agenda:        agenda.NewService(bills, financings, cards),
		Alelo:         alelo.NewService(conn),
		Receivables:   receivable.NewService(conn),
		Savings:       sav,
		Reports:       reports,
		Imports:       importer.NewService(conn, cards),
		Market:        markets,
		Notifications: notify.NewService(conn, agenda.NewService(bills, financings, cards), reports, sav, income.NewService(conn)),
		Sessions:      sessions,
		Backups:       backups,
		Version:       version,
		StartedAt:     time.Now(),
		Log:           log,
		Assets:        web.Assets(),
		Events:        hub,
	})
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	srv.RegisterOnShutdown(hub.Close)
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
	return account.NewService(conn), func() { _ = conn.Close() }, nil
}

// userCommand handles the "user add", "user pin" and "user list" subcommands.
func userCommand(args []string, log *slog.Logger) error {
	if len(args) == 0 {
		return errors.New("use: radar user add|pin|list")
	}
	sub, args := args[0], args[1:]
	fs := flag.NewFlagSet("user "+sub, flag.ContinueOnError)
	login := fs.String("login", "", "login do usuário")
	name := fs.String("name", "", "nome da pessoa")
	group := fs.String("group", "", "nome do grupo em que a pessoa entra")
	admin := fs.Bool("admin", false, "entra no grupo como administrador")
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
		u, err := accounts.Create(ctx, *login, *name)
		if err != nil {
			return err
		}
		fmt.Printf("Usuário %q criado para %s.\n", u.Login, u.Name)
		if *group != "" {
			return addToGroup(ctx, accounts, *group, u, *admin)
		}
	case "pin":
		u, err := accounts.GetByLogin(ctx, *login)
		if err != nil {
			return fmt.Errorf("usuário %q: %w", *login, err)
		}
		pin, err := readPIN()
		if err != nil {
			return err
		}
		if err := accounts.SetPIN(ctx, u.ID, pin); err != nil {
			return err
		}
		fmt.Printf("PIN de %q definido.\n", u.Login)
	case "list":
		users, err := accounts.List(ctx)
		if err != nil {
			return err
		}
		for _, u := range users {
			fmt.Printf("%d\t%s\t%s\n", u.ID, u.Login, u.Name)
		}
	default:
		return fmt.Errorf("subcomando desconhecido: user %s", sub)
	}
	return nil
}

// groupCommand handles the "group create", "group add" and "group list" subcommands (RF-78, RF-80).
func groupCommand(args []string, log *slog.Logger) error {
	if len(args) == 0 {
		return errors.New("use: radar group create|add|list")
	}
	sub, args := args[0], args[1:]
	fs := flag.NewFlagSet("group "+sub, flag.ContinueOnError)
	name := fs.String("name", "", "nome do grupo")
	adminLogin := fs.String("admin", "", "login do primeiro administrador")
	group := fs.String("group", "", "nome do grupo")
	login := fs.String("login", "", "login do usuário")
	asAdmin := fs.Bool("as-admin", false, "entra como administrador")
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
	case "create":
		u, err := accounts.GetByLogin(ctx, *adminLogin)
		if err != nil {
			return fmt.Errorf("administrador %q: %w", *adminLogin, err)
		}
		g, err := accounts.CreateGroup(ctx, *name, u.ID)
		if err != nil {
			return err
		}
		fmt.Printf("Grupo %q criado com %s como administrador.\n", g.Name, u.Name)
	case "add":
		u, err := accounts.GetByLogin(ctx, *login)
		if err != nil {
			return fmt.Errorf("usuário %q: %w", *login, err)
		}
		return addToGroup(ctx, accounts, *group, u, *asAdmin)
	case "list":
		groups, err := accounts.Groups(ctx)
		if err != nil {
			return err
		}
		for _, g := range groups {
			members, err := accounts.Members(ctx, g.ID)
			if err != nil {
				return err
			}
			fmt.Printf("%d\t%s\n", g.ID, g.Name)
			for _, m := range members {
				pin := "sem PIN"
				if m.HasPIN {
					pin = "com PIN"
				}
				fmt.Printf("\t%s\t%s\t%s\t%s\n", m.Login, m.Name, roleName(m.Role), pin)
			}
		}
	default:
		return fmt.Errorf("subcomando desconhecido: group %s", sub)
	}
	return nil
}

// addToGroup puts the user in the group with the given name.
func addToGroup(ctx context.Context, accounts *account.Service, groupName string, u account.User, admin bool) error {
	groups, err := accounts.Groups(ctx)
	if err != nil {
		return err
	}
	for _, g := range groups {
		if strings.EqualFold(g.Name, groupName) {
			role := account.RoleMember
			if admin {
				role = account.RoleAdmin
			}
			if err := accounts.AddMember(ctx, g.ID, u.ID, role); err != nil {
				return err
			}
			fmt.Printf("%s entrou no grupo %q como %s.\n", u.Name, g.Name, roleName(role))
			return nil
		}
	}
	return fmt.Errorf("grupo %q não encontrado; crie com radar group create", groupName)
}

// roleName writes a role in Portuguese for the terminal.
func roleName(r account.Role) string {
	if r == account.RoleAdmin {
		return "administrador"
	}
	return "membro"
}

// readPIN reads the PIN from the first line of the standard input, so it stays out of the shell history.
func readPIN() (string, error) {
	fmt.Fprint(os.Stderr, "PIN (4 a 6 números): ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
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
