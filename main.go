// synology-tui: a terminal UI for Synology DSM: system, storage and
// S.M.A.R.T., packages and logs, with a few actions.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"

	"github.com/ionut-maxim/synology-tui/dsm"
	"github.com/ionut-maxim/synology-tui/internal/ui"
)

var version = "dev" // set by the release build

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "synology-tui:", err)
		os.Exit(1)
	}
}

func run() error {
	host := flag.String("host", os.Getenv("SYNOLOGY_HOST"), "DSM URL, e.g. https://nas.local:5001 (env SYNOLOGY_HOST)")
	user := flag.String("user", os.Getenv("SYNOLOGY_USER"), "DSM account (env SYNOLOGY_USER); the password comes from SYNOLOGY_PASSWORD or a prompt")
	insecure := flag.Bool("insecure", os.Getenv("SYNOLOGY_INSECURE") == "true", "skip TLS verification: DSM's default certificate is self-signed (env SYNOLOGY_INSECURE=true)")
	status := flag.Bool("status", false, "print a status summary and exit")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return nil
	}
	if *host == "" || *user == "" {
		flag.Usage()
		return errors.New("-host and -user are required")
	}
	if !strings.Contains(*host, "://") {
		*host = "https://" + *host
	}
	password := os.Getenv("SYNOLOGY_PASSWORD")
	if password == "" {
		p, err := readPassword(fmt.Sprintf("DSM password for %s: ", *user))
		if err != nil {
			return err
		}
		password = p
	}

	client := dsm.New(*host, dsm.Options{Insecure: *insecure})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	err := client.Login(ctx, *user, password)
	cancel()
	if err != nil {
		return err
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = client.Logout(ctx)
	}()

	if *status {
		return ui.PrintStatus(context.Background(), os.Stdout, client)
	}
	_, err = tea.NewProgram(ui.New(client, *host)).Run()
	return err
}

func readPassword(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("no password: set SYNOLOGY_PASSWORD or run in a terminal")
	}
	fmt.Fprint(os.Stderr, prompt)
	b, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	return string(b), err
}
