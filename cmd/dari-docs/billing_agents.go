package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

func runBilling(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: dari-docs billing [balance|checkout]")
	}
	switch args[0] {
	case "balance":
		fs := flag.NewFlagSet("dari-docs billing balance", flag.ExitOnError)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		client, err := managedClientWithToken()
		if err != nil {
			return err
		}
		bal, err := client.Balance(context.Background())
		if err != nil {
			return err
		}
		fmt.Printf("%s balance: %s\n", bal.Email, formatCents(bal.BalanceCents))
		return nil
	case "checkout":
		fs := flag.NewFlagSet("dari-docs billing checkout", flag.ExitOnError)
		var amount string
		fs.StringVar(&amount, "amount", "", "credit purchase amount in dollars, for example 20 or 20.00")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		cents, err := parseDollarsToCents(amount)
		if err != nil {
			return err
		}
		client, err := managedClientWithToken()
		if err != nil {
			return err
		}
		checkout, err := client.CreateCheckout(context.Background(), cents)
		if err != nil {
			return err
		}
		if err := openBrowserURL(checkout.CheckoutURL); err != nil {
			fmt.Fprintf(os.Stderr, "Could not open browser automatically: %v\n", err)
		}
		fmt.Printf("Checkout URL: %s\n", checkout.CheckoutURL)
		return nil
	default:
		return fmt.Errorf("unknown billing command %q", args[0])
	}
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

func openBrowserURL(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
