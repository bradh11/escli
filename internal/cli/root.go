// Package cli defines the escli commands.
package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/bradh11/escli/internal/config"
	"github.com/bradh11/escli/internal/endpoint"
	"github.com/bradh11/escli/internal/es"
	"github.com/bradh11/escli/internal/secret"
	"github.com/bradh11/escli/internal/version"
	"github.com/spf13/cobra"
)

const rootLong = `escli is a read-only command-line client for Elasticsearch, built for people
and for AI agents.

Run "escli init" once to store the cluster address and your API key. After
that, every command prints JSON to stdout, and errors go to stderr with a
non-zero exit code.

Start with "escli indices" to see what exists, "escli mapping <index> --flat"
to see the fields, then "escli search" or "escli esql" to read data.

Environment variables:
  ESCLI_PROFILE     profile to use when --profile is not given
  ESCLI_URL         cluster address; overrides the profile (for CI)
  ESCLI_API_KEY     API key; overrides the stored key (for CI)
  ESCLI_CONFIG_DIR  configuration directory (default ~/.config/escli)`

type app struct {
	profile string
	compact bool
	timeout time.Duration

	in     io.Reader
	out    io.Writer
	errOut io.Writer
	reader *bufio.Reader
}

// Execute runs escli with the process arguments and returns the exit code.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	a := &app{in: os.Stdin, out: os.Stdout, errOut: os.Stderr}
	if err := a.rootCmd().ExecuteContext(ctx); err != nil {
		fmt.Fprintf(a.errOut, "error: %v\n", err)
		return 1
	}
	return 0
}

func (a *app) rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "escli",
		Short:         "Read-only Elasticsearch client for people and AI agents",
		Long:          rootLong,
		Version:       version.String(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetIn(a.in)
	root.SetOut(a.out)
	root.SetErr(a.errOut)

	flags := root.PersistentFlags()
	flags.StringVarP(&a.profile, "profile", "p", os.Getenv("ESCLI_PROFILE"), "profile to use (default: the current profile)")
	flags.BoolVar(&a.compact, "compact", false, "print JSON on one line instead of indented")
	flags.DurationVar(&a.timeout, "timeout", 60*time.Second, "time limit for each request")

	root.AddGroup(
		&cobra.Group{ID: "setup", Title: "Setup:"},
		&cobra.Group{ID: "read", Title: "Reading data:"},
	)
	for _, c := range []*cobra.Command{a.initCmd(), a.authCmd(), a.configCmd()} {
		c.GroupID = "setup"
		root.AddCommand(c)
	}
	for _, c := range []*cobra.Command{
		a.infoCmd(), a.indicesCmd(), a.mappingCmd(), a.searchCmd(),
		a.countCmd(), a.getCmd(), a.esqlCmd(), a.apiCmd(),
	} {
		c.GroupID = "read"
		root.AddCommand(c)
	}
	root.AddCommand(a.versionCmd())
	return root
}

func (a *app) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the escli version",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			_, err := fmt.Fprintln(a.out, "escli", version.String())
			return err
		},
	}
}

// connect builds a client from the selected profile, with ESCLI_URL and
// ESCLI_API_KEY taking precedence over what the profile stores.
func (a *app) connect() (*es.Client, error) {
	dir, err := config.Dir()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return nil, err
	}
	name, prof, found := cfg.Lookup(a.profile)

	rawURL := os.Getenv("ESCLI_URL")
	if rawURL == "" {
		if !found {
			return nil, notConfigured(name, a.profile != "")
		}
		rawURL = prof.URL
	}
	esURL, _, err := endpoint.Resolve(rawURL)
	if err != nil {
		return nil, err
	}

	key := os.Getenv("ESCLI_API_KEY")
	if key == "" {
		if !found {
			return nil, notConfigured(name, a.profile != "")
		}
		key, err = secret.New(dir).Get(name)
		if errors.Is(err, secret.ErrNotFound) {
			return nil, fmt.Errorf("%w for profile %q; run `escli init --profile %s`", err, name, name)
		}
		if err != nil {
			return nil, err
		}
	}
	if key, err = es.NormalizeAPIKey(key); err != nil {
		return nil, err
	}
	return es.New(esURL, key, a.timeout), nil
}

func notConfigured(name string, explicit bool) error {
	if explicit {
		return fmt.Errorf("profile %q does not exist; run `escli init --profile %s` or `escli config list`", name, name)
	}
	return errors.New("escli is not set up yet; run `escli init`")
}

// line reads one line of input, without the line ending.
func (a *app) line() (string, error) {
	if a.reader == nil {
		a.reader = bufio.NewReader(a.in)
	}
	s, err := a.reader.ReadString('\n')
	switch {
	case errors.Is(err, io.EOF) && s == "":
		return "", errors.New("no input")
	case err != nil && !errors.Is(err, io.EOF):
		return "", err
	}
	return strings.TrimSpace(s), nil
}
