package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/mupt-ai/dari-docs/internal/managed"
	"github.com/mupt-ai/dari-docs/internal/platformauth"
	"github.com/spf13/cobra"
)

func newAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "auth",
		Short:         "Authenticate to the managed service",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newAuthLoginCommand(),
		newAuthLogoutCommand(),
		newAuthStatusCommand(),
		newAuthAPIKeyCommand(),
	)
	return cmd
}

func newAuthLoginCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "login",
		Short:         "Log in with a browser",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthLogin(cmd.Context())
		},
	}
}

func newAuthLogoutCommand() *cobra.Command {
	var all bool
	var interactiveOnly bool
	var automationOnly bool
	cmd := &cobra.Command{
		Use:           "logout",
		Short:         "Log out or revoke credentials",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if (interactiveOnly || automationOnly) && !all {
				return fmt.Errorf("--interactive-only and --automation-only require --all")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthLogoutOptions(cmd.Context(), all, interactiveOnly, automationOnly)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "revoke all managed service credentials for this account")
	cmd.Flags().BoolVar(&interactiveOnly, "interactive-only", false, "with --all, revoke only browser-login sessions")
	cmd.Flags().BoolVar(&automationOnly, "automation-only", false, "with --all, revoke only API keys")
	cmd.MarkFlagsMutuallyExclusive("interactive-only", "automation-only")
	return cmd
}

func newAuthStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "status",
		Short:         "Show authenticated account",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthStatus(cmd.Context())
		},
	}
}

func newAuthAPIKeyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "api-key",
		Aliases:       []string{"api-keys", "token"},
		Short:         "Manage API keys",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newAuthTokenCreateCommand(),
		newAuthTokenListCommand(),
		newAuthTokenRevokeCommand(),
	)
	return cmd
}

func newAuthTokenCreateCommand() *cobra.Command {
	var name string
	var scopes []string
	var expiresIn string
	cmd := &cobra.Command{
		Use:           "create",
		Short:         "Create an API key",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthTokenCreate(cmd.Context(), name, scopes, expiresIn)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "API key name, for example github-actions")
	cmd.Flags().StringSliceVar(&scopes, "scope", nil, "API key scope; repeatable or comma-separated (default: managed:read, managed:check, and managed:optimize)")
	cmd.Flags().StringVar(&expiresIn, "expires-in", "", "optional expiration such as 90d or 24h")
	return cmd
}

func newAuthTokenListCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "list",
		Short:         "List API keys",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthTokenList(cmd.Context())
		},
	}
}

func newAuthTokenRevokeCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "revoke <api-key-id>",
		Short:         "Revoke an API key",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthTokenRevoke(cmd.Context(), args[0])
		},
	}
}

func runAuthLogout(args []string) error {
	cmd := newAuthLogoutCommand()
	cmd.SetArgs(args)
	return cmd.Execute()
}

func runAuthLogin(ctx context.Context) error {
	if token, err := managed.LoadToken(managed.DefaultBaseURL); err != nil {
		return err
	} else if strings.TrimSpace(token) != "" {
		client := managed.NewWithAuthToken(managed.DefaultBaseURL, managed.AuthToken{Token: token, Source: managed.AuthSourceLocal})
		me, err := client.Me(ctx)
		if err == nil {
			fmt.Printf("Already logged in to %s as %s\n", managed.DefaultBaseURL, me.Email)
			return nil
		}
		var httpErr *managed.HTTPError
		if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusUnauthorized {
			return err
		}
		if err := managed.DeleteToken(managed.DefaultBaseURL); err != nil {
			return err
		}
	}
	verified, err := exchangeManagedBrowserLogin(ctx)
	if err != nil {
		return err
	}
	if err := managed.SaveToken(managed.DefaultBaseURL, verified.Token); err != nil {
		return err
	}
	fmt.Printf("Logged in to %s as %s\n", managed.DefaultBaseURL, verified.Email)
	return nil
}

func exchangeManagedBrowserLogin(ctx context.Context) (managed.DariExchangeResponse, error) {
	authConfig, err := platformauth.FetchConfig(ctx, "https://api.dari.dev")
	if err != nil {
		return managed.DariExchangeResponse{}, err
	}
	session, err := platformauth.LoginWithBrowser(ctx, authConfig, os.Stdin, os.Stderr)
	if err != nil {
		return managed.DariExchangeResponse{}, err
	}
	client := managed.New(managed.DefaultBaseURL, "")
	return client.ExchangeDariToken(ctx, session.AccessToken)
}

func runAuthLogoutOptions(ctx context.Context, all bool, interactiveOnly bool, automationOnly bool) error {
	if all {
		kind := ""
		switch {
		case interactiveOnly:
			kind = "interactive"
		case automationOnly:
			kind = "automation"
		}
		return runAuthLogoutAll(ctx, kind)
	}
	auth, err := managed.LoadAuthToken(managed.DefaultBaseURL)
	if err != nil {
		return err
	}
	if auth.Token == "" {
		fmt.Printf("Already logged out locally.\nTo revoke server-side credentials from other devices or deleted local sessions, run `dari-docs auth logout --all`.\n")
		return nil
	}
	client := managed.NewWithAuthToken(managed.DefaultBaseURL, auth)
	if err := client.Logout(ctx); err != nil {
		var httpErr *managed.HTTPError
		var invalidEnv *managed.InvalidEnvTokenError
		if errors.As(err, &invalidEnv) {
			return err
		}
		if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusUnauthorized {
			return err
		}
	}
	if auth.Source == managed.AuthSourceLocal {
		if err := managed.DeleteToken(managed.DefaultBaseURL); err != nil {
			return err
		}
		fmt.Printf("Logged out of %s\n", managed.DefaultBaseURL)
	} else {
		fmt.Printf("Revoked API key from %s. Unset %s to stop using it locally.\n", managed.EnvTokenName, managed.EnvTokenName)
	}
	return nil
}

func runAuthLogoutAll(ctx context.Context, kind string) error {
	auth, err := managed.LoadAuthToken(managed.DefaultBaseURL)
	if err != nil {
		return err
	}
	if auth.Token != "" {
		client := managed.NewWithAuthToken(managed.DefaultBaseURL, auth)
		if err := client.LogoutAllKind(ctx, kind); err == nil {
			if auth.Source == managed.AuthSourceLocal && kind != "automation" {
				if err := managed.DeleteToken(managed.DefaultBaseURL); err != nil {
					return err
				}
			}
			fmt.Printf("%s.\n", logoutAllMessage(kind))
			return nil
		} else {
			var httpErr *managed.HTTPError
			var invalidEnv *managed.InvalidEnvTokenError
			if errors.As(err, &invalidEnv) {
				return err
			}
			if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusUnauthorized {
				return err
			}
			if auth.Source == managed.AuthSourceLocal {
				if err := managed.DeleteToken(managed.DefaultBaseURL); err != nil {
					return err
				}
				fmt.Fprintln(os.Stderr, "Stored login was invalid; re-authenticating to revoke server-side credentials.")
			} else {
				return err
			}
		}
	} else {
		fmt.Fprintln(os.Stderr, "No local login found; re-authenticating to revoke server-side credentials.")
	}
	verified, err := exchangeManagedBrowserLogin(ctx)
	if err != nil {
		return err
	}
	client := managed.New(managed.DefaultBaseURL, verified.Token)
	if err := client.LogoutAllKind(ctx, kind); err != nil {
		return err
	}
	if kind == "automation" {
		if err := client.Logout(ctx); err != nil {
			return err
		}
	}
	fmt.Printf("%s for %s.\n", logoutAllMessage(kind), verified.Email)
	return nil
}

func logoutAllMessage(kind string) string {
	switch kind {
	case "interactive":
		return "Revoked all interactive Dari Docs managed sessions"
	case "automation":
		return "Revoked all Dari Docs API keys"
	default:
		return "Revoked all Dari Docs managed credentials"
	}
}

func runAuthStatus(ctx context.Context) error {
	client, auth, err := managedClientWithAuth()
	if err != nil {
		return err
	}
	me, err := client.Me(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Authenticated to %s\n", managed.DefaultBaseURL)
	fmt.Printf("Email: %s\n", me.Email)
	fmt.Printf("Source: %s\n", authSourceLabel(auth.Source))
	if me.Token.ID != "" {
		name := me.Token.Name
		if name == "" {
			name = me.Token.ID
		}
		fmt.Printf("Credential: %s (%s)\n", name, me.Token.Kind)
	}
	if len(me.Token.Scopes) > 0 {
		fmt.Printf("Scopes: %s\n", strings.Join(me.Token.Scopes, ", "))
	}
	return nil
}

func runAuthTokenCreate(ctx context.Context, name string, scopes []string, expiresIn string) error {
	expiresAt, err := parseExpiresIn(expiresIn)
	if err != nil {
		return err
	}
	client, _, err := managedClientWithAuth()
	if err != nil {
		return err
	}
	tokenScopes := uniqueTrimmedList(scopes)
	if len(tokenScopes) == 0 {
		tokenScopes = []string{"managed:read", "managed:check", "managed:optimize"}
	}
	resp, err := client.CreateAuthToken(ctx, managed.TokenCreateRequest{
		Name:      name,
		Scopes:    tokenScopes,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return err
	}
	displayName := resp.Name
	if displayName == "" {
		displayName = resp.ID
	}
	fmt.Printf("Created API key %q.\n\n", displayName)
	fmt.Printf("%s=%s\n\n", managed.EnvTokenName, resp.Token)
	fmt.Println("Copy this value now. It will not be shown again.")
	return nil
}

func runAuthTokenList(ctx context.Context) error {
	client, _, err := managedClientWithAuth()
	if err != nil {
		return err
	}
	resp, err := client.ListAuthTokens(ctx)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tKIND\tSCOPES\tLAST USED\tEXPIRES")
	for _, token := range resp.Tokens {
		name := token.Name
		if name == "" {
			name = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			token.ID,
			name,
			token.Kind,
			strings.Join(token.Scopes, ","),
			formatOptionalTime(token.LastUsedAt),
			formatOptionalTime(token.ExpiresAt),
		)
	}
	return tw.Flush()
}

func runAuthTokenRevoke(ctx context.Context, tokenID string) error {
	client, _, err := managedClientWithAuth()
	if err != nil {
		return err
	}
	if err := client.RevokeAuthToken(ctx, tokenID); err != nil {
		return err
	}
	fmt.Printf("Revoked API key %s\n", tokenID)
	return nil
}
