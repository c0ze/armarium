package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/c0ze/armarium/internal/auth"
	"github.com/c0ze/armarium/internal/config"
	"github.com/c0ze/armarium/internal/store"
)

// healthcheck probes /healthz on the listen address from ARMARIUM_LISTEN. The
// scratch image has no curl, so Docker's HEALTHCHECK runs this instead.
func healthcheck() error {
	addr := os.Getenv("ARMARIUM_LISTEN")
	if addr == "" {
		addr = config.Default().Listen
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	c := http.Client{Timeout: 3 * time.Second}
	res, err := c.Get("http://" + net.JoinHostPort(host, port) + "/healthz")
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz returned %d", res.StatusCode)
	}
	return nil
}

// readPassword prompts on stderr and reads one line from stdin, hidden when
// stdin is a terminal.
func readPassword(prompt string, in *bufio.Reader) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	restore := hideInput()
	line, err := in.ReadString('\n')
	restore()
	fmt.Fprintln(os.Stderr)
	if err != nil && line == "" {
		return "", errors.New("no password on stdin")
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func hashPassword() error {
	pw, err := readPassword("password: ", bufio.NewReader(os.Stdin))
	if err != nil {
		return err
	}
	if len(pw) < 8 {
		return errors.New("use at least 8 characters")
	}
	h, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	fmt.Println(h)
	return nil
}

// password sets the owner password stored in the database, for a forgotten
// password or a server set up without the browser.
func (a *app) password(ctx context.Context) error {
	in := bufio.NewReader(os.Stdin)
	pw, err := readPassword("new password: ", in)
	if err != nil {
		return err
	}
	again, err := readPassword("again: ", in)
	if err != nil {
		return err
	}
	if pw != again {
		return errors.New("the two passwords don't match")
	}
	if err := a.SetPassword(ctx, pw); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Password set. Everyone logged in to the web UI has to log in again.")
	return nil
}

func (a *app) token(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("token add <name> | token list | token rm <name>")
	}
	switch args[0] {
	case "add":
		if len(args) != 2 {
			return errors.New("token add <name>")
		}
		secret := auth.NewSecret()
		if _, err := a.Store.AddToken(ctx, args[1], auth.Digest(secret)); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "token %q created; it is shown only once:\n", args[1])
		fmt.Println(secret)
	case "list":
		ts, err := a.Store.Tokens(ctx)
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tCREATED\tLAST USED")
		for _, t := range ts {
			last := "never"
			if t.LastUsedAt != nil {
				last = time.Unix(*t.LastUsedAt, 0).Format(time.DateTime)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", t.Name, time.Unix(t.CreatedAt, 0).Format(time.DateTime), last)
		}
		return w.Flush()
	case "rm":
		if len(args) != 2 {
			return errors.New("token rm <name>")
		}
		ts, err := a.Store.Tokens(ctx)
		if err != nil {
			return err
		}
		for _, t := range ts {
			if t.Name == args[1] {
				return a.Store.DeleteToken(ctx, t.ID)
			}
		}
		return fmt.Errorf("no token named %q", args[1])
	default:
		return fmt.Errorf("unknown token command %q", args[0])
	}
	return nil
}

func (a *app) library(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("library list | library rename <old> <new> | library rm <name>")
	}
	switch args[0] {
	case "list":
		libs, err := a.Store.Libraries(ctx)
		if err != nil {
			return err
		}
		configured := map[string]bool{}
		for _, l := range a.Cfg.Libraries {
			configured[l.Name] = true
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tCONFIGURED\tITEMS")
		for _, l := range libs {
			_, n, err := a.Store.Items(ctx, store.ItemFilter{LibraryID: l.ID, Limit: 1})
			if err != nil {
				return err
			}
			yes := "no"
			if configured[l.Name] {
				yes = "yes"
			}
			fmt.Fprintf(w, "%s\t%s\t%d\n", l.Name, yes, n)
		}
		return w.Flush()
	case "rename":
		if len(args) != 3 {
			return errors.New("library rename <old> <new>")
		}
		if err := a.Store.RenameLibrary(ctx, args[1], args[2]); err != nil {
			return fmt.Errorf("rename %q: %w", args[1], err)
		}
		fmt.Printf("renamed %q to %q\n", args[1], args[2])
	case "rm":
		if len(args) != 2 {
			return errors.New("library rm <name>")
		}
		n, err := a.Store.RemoveLibrary(ctx, args[1])
		if err != nil {
			return fmt.Errorf("remove %q: %w", args[1], err)
		}
		fmt.Printf("removed %q and %d items\n", args[1], n)
	default:
		return fmt.Errorf("unknown library command %q", args[0])
	}
	return nil
}

// hideInput turns terminal echo off while a password is typed, and back on even
// when the user presses Ctrl-C: Go's default SIGINT exit would skip a defer and
// leave the terminal silent.
func hideInput() func() {
	if fi, err := os.Stdin.Stat(); err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return func() {} // piped (docker run -i): nothing is echoed anyway
	}
	stty := func(arg string) error {
		c := exec.Command("stty", arg)
		c.Stdin = os.Stdin
		return c.Run()
	}
	if stty("-echo") != nil {
		return func() {}
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case <-sig:
			stty("echo")
			fmt.Fprintln(os.Stderr)
			os.Exit(130)
		case <-done:
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			signal.Stop(sig)
			stty("echo")
		})
	}
}
