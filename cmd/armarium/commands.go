package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/c0ze/armarium/internal/auth"
	"github.com/c0ze/armarium/internal/config"
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

func hashPassword() error {
	fmt.Fprint(os.Stderr, "password: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return errors.New("no password on stdin")
	}
	pw := strings.TrimRight(line, "\r\n")
	if len(pw) < 8 {
		return errors.New("use at least 8 characters")
	}
	h, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr)
	fmt.Println(h)
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
		if _, err := a.store.AddToken(ctx, args[1], auth.Digest(secret)); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "token %q created; it is shown only once:\n", args[1])
		fmt.Println(secret)
	case "list":
		ts, err := a.store.Tokens(ctx)
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
		ts, err := a.store.Tokens(ctx)
		if err != nil {
			return err
		}
		for _, t := range ts {
			if t.Name == args[1] {
				return a.store.DeleteToken(ctx, t.ID)
			}
		}
		return fmt.Errorf("no token named %q", args[1])
	default:
		return fmt.Errorf("unknown token command %q", args[0])
	}
	return nil
}
