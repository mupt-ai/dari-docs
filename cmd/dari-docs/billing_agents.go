package main

import (
	"context"
	"fmt"
	"os"

	"github.com/mupt-ai/dari-docs/internal/browser"
	"github.com/spf13/cobra"
)

func newBillingCommand() *cobra.Command {
	return newUnsupportedManagedCommand("billing [command]")
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
	_ = cmd.MarkFlagRequired("amount")
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

func newAgentsCommand() *cobra.Command {
	return newUnsupportedManagedCommand("agents [command]")
}
