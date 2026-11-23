package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/bradh11/escli/internal/config"
	"github.com/bradh11/escli/internal/endpoint"
	"github.com/bradh11/escli/internal/es"
	"github.com/bradh11/escli/internal/secret"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const initLong = `Set up a profile: the address of your Elasticsearch cluster and your API key.

You can give the address as the Elasticsearch URL, the Kibana URL (escli
converts it) or the Elastic Cloud ID. escli checks the key against the cluster
before it saves anything. The address goes to the configuration file; the API
key goes to your system keychain.

Create your own API key in Kibana under Stack Management > Security > API keys.
Give it read privileges only (see the README) and an expiry date.

Run it again to replace an expired key. Use --profile to set up a second cluster.`

func (a *app) initCmd() *cobra.Command {
	var (
		rawURL   string
		keyStdin bool
		noVerify bool
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Set up the cluster address and your API key",
		Long:  initLong,
		Example: `  escli init
  escli init --profile staging --url https://my-project.kb.us-east-2.aws.elastic.cloud
  echo "$KEY" | escli init --url "$URL" --api-key-stdin`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := config.Dir()
			if err != nil {
				return err
			}
			cfg, err := config.Load(dir)
			if err != nil {
				return err
			}
			name := a.profile
			if name == "" {
				name = config.DefaultProfile
			}
			if err := config.ValidateProfileName(name); err != nil {
				return err
			}
			existing, exists := cfg.Profiles[name]

			if rawURL == "" {
				prompt := "Elasticsearch URL, Kibana URL or Cloud ID"
				if exists {
					prompt += " [" + existing.URL + "]"
				}
				fmt.Fprint(a.errOut, prompt+": ")
				if rawURL, err = a.line(); err != nil {
					return err
				}
				if rawURL == "" && exists {
					rawURL = existing.URL
				}
			}
			esURL, note, err := endpoint.Resolve(rawURL)
			if err != nil {
				return err
			}
			if note != "" {
				fmt.Fprintf(a.errOut, "Note: escli %s.\n", note)
			}
			fmt.Fprintf(a.errOut, "Elasticsearch address: %s\n", esURL)

			var key string
			if keyStdin {
				data, err := io.ReadAll(a.in)
				if err != nil {
					return err
				}
				key = string(data)
			} else {
				fmt.Fprintln(a.errOut, "\nPaste your API key. To create one, open Kibana > Stack Management > Security > API keys:")
				if kb := endpoint.KibanaURL(esURL); kb != "" {
					fmt.Fprintf(a.errOut, "  %s/app/management/security/api_keys\n", kb)
				}
				if key, err = a.secretLine("API key (input hidden): "); err != nil {
					return err
				}
			}
			if key, err = es.NormalizeAPIKey(key); err != nil {
				return err
			}

			if !noVerify {
				who, err := whoAmI(cmd.Context(), es.New(esURL, key, a.timeout))
				if err != nil {
					return fmt.Errorf("the key did not work, so nothing was saved: %w", err)
				}
				fmt.Fprintf(a.errOut, "Signed in as %s.\n", who)
			}

			where, err := secret.New(dir).Set(name, key)
			if err != nil {
				return fmt.Errorf("store the API key: %w", err)
			}
			cfg.Profiles[name] = config.Profile{URL: esURL}
			cfg.Current = name
			if err := cfg.Save(); err != nil {
				return err
			}
			fmt.Fprintf(a.errOut, "Saved profile %q in %s and the API key in %s.\n", name, cfg.Path(), where)
			fmt.Fprintf(a.errOut, "Profile %q is now the current profile. Try: escli indices\n", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&rawURL, "url", "", "cluster address: Elasticsearch URL, Kibana URL or Cloud ID")
	cmd.Flags().BoolVar(&keyStdin, "api-key-stdin", false, "read the API key from stdin instead of prompting")
	cmd.Flags().BoolVar(&noVerify, "no-verify", false, "save without checking the key against the cluster")
	return cmd
}

// secretLine prompts for a value without echoing it when stdin is a terminal.
func (a *app) secretLine(prompt string) (string, error) {
	fmt.Fprint(a.errOut, prompt)
	if f, ok := a.in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		data, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(a.errOut)
		return string(data), err
	}
	return a.line()
}

// whoAmI returns a short description of the identity behind the client's key.
func whoAmI(ctx context.Context, c *es.Client) (string, error) {
	info, err := authenticate(ctx, c)
	if err != nil {
		return "", err
	}
	who := info.Username
	if info.APIKey.Name != "" {
		who += fmt.Sprintf(" (API key %q)", info.APIKey.Name)
	}
	return who, nil
}

type authInfo struct {
	Username string `json:"username"`
	Realm    struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"authentication_realm"`
	AuthType string `json:"authentication_type"`
	APIKey   struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"api_key"`
	Roles []string `json:"roles"`
}

func authenticate(ctx context.Context, c *es.Client) (*authInfo, error) {
	body, err := c.Do(ctx, http.MethodGet, "/_security/_authenticate", nil, nil)
	if err != nil {
		return nil, err
	}
	var info authInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("read the _authenticate response: %w", err)
	}
	return &info, nil
}

func (a *app) authCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Check or remove the stored API key",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show who the API key signs in as",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := a.connect()
			if err != nil {
				return err
			}
			info, err := authenticate(cmd.Context(), c)
			if err != nil {
				return err
			}
			out := map[string]any{
				"url":                 c.BaseURL(),
				"username":            info.Username,
				"authentication_type": info.AuthType,
				"realm":               info.Realm.Name,
			}
			if info.APIKey.Name != "" || info.APIKey.ID != "" {
				out["api_key"] = info.APIKey
			}
			if len(info.Roles) > 0 {
				out["roles"] = info.Roles
			}
			return a.writeJSON(out)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "logout",
		Short: "Delete the stored API key of the profile",
		Long:  "Delete the stored API key of the profile. The key stays valid in Elasticsearch; revoke it in Kibana if it may have leaked.",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			dir, err := config.Dir()
			if err != nil {
				return err
			}
			cfg, err := config.Load(dir)
			if err != nil {
				return err
			}
			name, _, _ := cfg.Lookup(a.profile)
			if err := secret.New(dir).Delete(name); err != nil {
				if errors.Is(err, secret.ErrNotFound) {
					return fmt.Errorf("%w for profile %q", err, name)
				}
				return err
			}
			fmt.Fprintf(a.errOut, "Deleted the API key of profile %q.\n", name)
			return nil
		},
	})
	return cmd
}

func (a *app) configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "List profiles or change the current profile",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List the profiles",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			dir, err := config.Dir()
			if err != nil {
				return err
			}
			cfg, err := config.Load(dir)
			if err != nil {
				return err
			}
			current, _, _ := cfg.Lookup("")
			type row struct {
				Name    string `json:"name"`
				URL     string `json:"url"`
				Current bool   `json:"current"`
			}
			rows := []row{}
			for name, p := range cfg.Profiles {
				rows = append(rows, row{Name: name, URL: p.URL, Current: name == current})
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
			return a.writeJSON(map[string]any{"config_file": cfg.Path(), "profiles": rows})
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "use <profile>",
		Short: "Make a profile the current profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			dir, err := config.Dir()
			if err != nil {
				return err
			}
			cfg, err := config.Load(dir)
			if err != nil {
				return err
			}
			if _, ok := cfg.Profiles[args[0]]; !ok {
				names := make([]string, 0, len(cfg.Profiles))
				for n := range cfg.Profiles {
					names = append(names, n)
				}
				sort.Strings(names)
				return fmt.Errorf("profile %q does not exist (profiles: %s)", args[0], strings.Join(names, ", "))
			}
			cfg.Current = args[0]
			if err := cfg.Save(); err != nil {
				return err
			}
			fmt.Fprintf(a.errOut, "Profile %q is now the current profile.\n", args[0])
			return nil
		},
	})
	return cmd
}
