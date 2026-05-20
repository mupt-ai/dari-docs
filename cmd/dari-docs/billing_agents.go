package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/mupt-ai/dari-docs/internal/browser"
	"github.com/spf13/cobra"
)

func newBillingCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "billing",
		Short:         "Manage managed-service billing",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newBillingBalanceCommand(), newBillingCheckoutCommand())
	return cmd
}

func newBillingBalanceCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "balance",
		Short:         "Show credit balance",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBillingBalance(cmd.Context())
		},
	}
}

func newBillingCheckoutCommand() *cobra.Command {
	var amount string
	cmd := &cobra.Command{
		Use:           "checkout",
		Short:         "Buy credits",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBillingCheckout(cmd.Context(), amount)
		},
	}
	cmd.Flags().StringVar(&amount, "amount", "", "credit purchase amount in dollars, for example 20 or 20.00")
	return cmd
}

func runBillingBalance(ctx context.Context) error {
	client, err := managedClientWithToken()
	if err != nil {
		return err
	}
	bal, err := client.Balance(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("%s balance: %s\n", bal.Email, formatCents(bal.BalanceCents))
	return nil
}

func runBillingCheckout(ctx context.Context, amount string) error {
	cents, err := parseDollarsToCents(amount)
	if err != nil {
		return err
	}
	client, err := managedClientWithToken()
	if err != nil {
		return err
	}
	checkout, err := client.CreateCheckout(ctx, cents)
	if err != nil {
		return err
	}
	if err := browser.Open(checkout.CheckoutURL); err != nil {
		fmt.Fprintf(os.Stderr, "Could not open browser automatically: %v\n", err)
	}
	fmt.Printf("Checkout URL: %s\n", checkout.CheckoutURL)
	return nil
}

func runAgents(args []string) error {
	if len(args) == 0 || args[0] != "deploy" {
		return fmt.Errorf("managed mode uses hosted Dari Docs agents automatically. For self-managed agents, run `dari-docs init --deploy`")
	}
	fs := flag.NewFlagSet("dari-docs agents deploy", flag.ExitOnError)
	var managedMode bool
	fs.BoolVar(&managedMode, "managed", false, "use hosted Dari Docs managed agents")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if !managedMode {
		return fmt.Errorf("for self-managed agents, run `dari-docs init --deploy`")
	}
	fmt.Println("Managed mode uses hosted Dari Docs agents automatically.")
	return nil
}
