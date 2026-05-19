import { useEffect, useMemo, useState } from "react";
import { Plus, X } from "lucide-react";
import * as DialogPrimitive from "@radix-ui/react-dialog";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  createCheckout,
  getBalance,
  getBillingConfig,
  getRunConfig,
  type BillingConfig,
  type RunConfig,
} from "@/lib/billing";
import { cn, formatCents } from "@/lib/utils";

const fallbackBillingConfig: BillingConfig = {
  min_checkout_cents: 500,
  default_checkout_cents: 500,
  max_checkout_cents: 50000,
};

export default function Usage() {
  const [balance, setBalance] = useState<number | null>(null);
  const [granted, setGranted] = useState<number | null>(null);
  const [spent, setSpent] = useState<number | null>(null);
  const [config, setConfig] = useState<RunConfig | null>(null);
  const [billingConfig, setBillingConfig] = useState<BillingConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [buyOpen, setBuyOpen] = useState(false);
  const [amount, setAmount] = useState(centsToDollars(fallbackBillingConfig.default_checkout_cents));
  const [checkoutError, setCheckoutError] = useState<string | null>(null);
  const [checkingOut, setCheckingOut] = useState(false);

  useEffect(() => {
    let cancelled = false;
    Promise.all([getBalance(), getRunConfig(), getBillingConfig()])
      .then(([bal, cfg, billingCfg]) => {
        if (cancelled) return;
        setBalance(bal.balance_cents);
        setGranted(bal.credit_granted_cents ?? null);
        setSpent(bal.credit_spent_cents ?? null);
        setConfig(cfg);
        setBillingConfig(billingCfg);
        setAmount(centsToDollars(billingCfg.default_checkout_cents));
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const amountCents = useMemo(() => {
    const parsed = Number(amount);
    if (!Number.isFinite(parsed)) return 0;
    return Math.round(parsed * 100);
  }, [amount]);

  const activeBillingConfig = billingConfig ?? fallbackBillingConfig;
  const balanceCents = balance ?? 0;
  const grantedCents = granted ?? balanceCents;
  const spentCents = spent ?? 0;
  const usedPct = grantedCents > 0 ? Math.min(100, Math.max(0, (spentCents / grantedCents) * 100)) : 0;
  const exhausted = !loading && !error && balanceCents <= 0;
  const invalid =
    !billingConfig ||
    amountCents < activeBillingConfig.min_checkout_cents ||
    amountCents > activeBillingConfig.max_checkout_cents ||
    !/^\d+(\.\d{1,2})?$/.test(amount.trim());

  const startCheckout = async () => {
    if (invalid) {
      setCheckoutError(
        `Enter an amount between ${formatCents(activeBillingConfig.min_checkout_cents)} and ${formatCents(activeBillingConfig.max_checkout_cents)}.`
      );
      return;
    }
    setCheckingOut(true);
    setCheckoutError(null);
    try {
      const checkout = await createCheckout(amountCents);
      window.location.assign(checkout.checkout_url);
    } catch (e) {
      setCheckoutError(e instanceof Error ? e.message : String(e));
      setCheckingOut(false);
    }
  };

  return (
    <div className="px-6 py-6">
      <div className="mb-6">
        <h1 className="text-xl font-medium">Usage</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Spend and remaining credit for dari-docs.
        </p>
      </div>

      {error && (
        <div className="mb-6 border border-destructive/50 bg-destructive/10 p-3 text-sm">
          {error}
        </div>
      )}

      {loading ? (
        <div className="text-sm text-muted-foreground">loading…</div>
      ) : (
        <div className="flex flex-col gap-6">
          <Card className={exhausted ? "border-destructive/50" : ""}>
            <CardHeader className="flex flex-row items-center justify-between gap-4 space-y-0">
              <CardTitle>Credit Balance</CardTitle>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={!billingConfig}
                onClick={() => {
                  setCheckoutError(null);
                  setBuyOpen(true);
                }}
                className="shrink-0 bg-white text-black hover:bg-white/90"
              >
                <Plus className="mr-1 h-4 w-4" />
                Buy Credits
              </Button>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <div>
                <div className="text-3xl font-medium tracking-tight">
                  {formatCents(balanceCents)}
                </div>
                <div className="mt-1 text-xs text-muted-foreground">
                  remaining of {formatCents(grantedCents)} granted
                </div>
              </div>
              <div className="h-2 w-full overflow-hidden rounded-full bg-muted">
                <div
                  className={exhausted ? "h-full bg-destructive" : "h-full bg-brand"}
                  style={{ width: `${usedPct}%` }}
                />
              </div>
              <div className="text-xs text-muted-foreground">
                {formatCents(spentCents)} spent ({usedPct.toFixed(1)}%)
              </div>
              {exhausted && (
                <div className="border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive-foreground">
                  Your account is out of credit. New managed runs will be
                  rejected until more credit is added.
                </div>
              )}
            </CardContent>
          </Card>

          <BuyCreditsDialog
            open={buyOpen}
            onOpenChange={setBuyOpen}
            billingConfig={billingConfig}
            creditAmount={amount}
            onCreditAmountChange={setAmount}
            minimumAmountCents={activeBillingConfig.min_checkout_cents}
            maximumAmountCents={activeBillingConfig.max_checkout_cents}
            checkingOut={checkingOut}
            checkoutError={checkoutError}
            onCheckout={startCheckout}
          />

          <Card>
            <CardHeader>
              <CardTitle>Breakdown</CardTitle>
              <CardDescription>Managed run guardrails and reserves.</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-2 text-xs text-muted-foreground">
              <Row
                label="Active Runs"
                value={`${config?.max_active_runs_per_user ?? "—"} Per Account`}
              />
              <Row
                label="Tasks Per Run"
                value={`${config?.max_tasks_per_run ?? "—"} Max`}
              />
              <Row
                label="Tester Reserve"
                value={formatCents(config?.tester_session_reserve_cents ?? 0)}
              />
              <Row
                label="Editor Reserve"
                value={formatCents(config?.editor_session_reserve_cents ?? 0)}
              />
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  );
}

function BuyCreditsDialog({
  open,
  onOpenChange,
  billingConfig,
  creditAmount,
  onCreditAmountChange,
  minimumAmountCents,
  maximumAmountCents,
  checkingOut,
  checkoutError,
  onCheckout,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  billingConfig: BillingConfig | null;
  creditAmount: string;
  onCreditAmountChange: (value: string) => void;
  minimumAmountCents: number;
  maximumAmountCents: number;
  checkingOut: boolean;
  checkoutError: string | null;
  onCheckout: () => void;
}) {
  const handleOpenChange = (nextOpen: boolean) => {
    if (checkingOut && !nextOpen) return;
    onOpenChange(nextOpen);
  };

  return (
    <DialogPrimitive.Root open={open} onOpenChange={handleOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay
          className={cn(
            "fixed inset-0 z-50 bg-black/70 backdrop-blur-sm",
            "data-[state=open]:animate-in data-[state=closed]:animate-out",
            "data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
          )}
        />
        <DialogPrimitive.Content
          className={cn(
            "fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2",
            "border border-border bg-card p-6 shadow-lg focus:outline-none",
            "data-[state=open]:animate-in data-[state=closed]:animate-out",
            "data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0",
            "data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95"
          )}
        >
          <div className="flex items-start justify-between gap-4">
            <div>
              <DialogPrimitive.Title className="text-base font-medium text-foreground">
                Buy Dari Credits
              </DialogPrimitive.Title>
              <DialogPrimitive.Description className="mt-2 text-sm leading-6 text-muted-foreground">
                Credits are added to dari-docs after Stripe confirms payment.
              </DialogPrimitive.Description>
            </div>
            <button
              type="button"
              aria-label="Close"
              onClick={() => onOpenChange(false)}
              disabled={checkingOut}
              className="-mr-2 -mt-2 inline-flex h-8 w-8 shrink-0 items-center justify-center text-muted-foreground hover:bg-accent hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
            >
              <X className="h-4 w-4" />
            </button>
          </div>

          <div className="mt-5 space-y-4">
            {billingConfig ? (
              <div className="space-y-2">
                <div className="flex items-center justify-between gap-3">
                  <label htmlFor="credit-amount" className="text-sm font-medium">
                    Credits to buy
                  </label>
                  <span className="text-xs text-muted-foreground">
                    {formatCents(minimumAmountCents)}–{formatCents(maximumAmountCents)}
                  </span>
                </div>
                <Input
                  id="credit-amount"
                  type="number"
                  min={String(billingConfig.min_checkout_cents / 100)}
                  max={String(billingConfig.max_checkout_cents / 100)}
                  step="0.01"
                  value={creditAmount}
                  onChange={(event) => onCreditAmountChange(event.target.value)}
                  disabled={checkingOut}
                />
                <p className="text-xs leading-5 text-muted-foreground">
                  Choose an amount in USD. Checkout is handled securely by
                  Stripe.
                </p>
              </div>
            ) : (
              <div className="border border-border bg-muted/40 p-3 text-sm text-muted-foreground">
                Credit purchases are not currently configured.
              </div>
            )}

            {checkoutError && (
              <div className="border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive-foreground">
                {checkoutError}
              </div>
            )}
          </div>

          <div className="mt-6 flex justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={checkingOut}
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <Button
              type="button"
              size="sm"
              disabled={checkingOut || !billingConfig}
              onClick={onCheckout}
            >
              {checkingOut ? "Checkout…" : "Checkout"}
            </Button>
          </div>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}

function centsToDollars(cents: number): string {
  return (cents / 100).toFixed(2);
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between">
      <span className="uppercase tracking-widest">{label}</span>
      <span className="text-foreground">{value}</span>
    </div>
  );
}
