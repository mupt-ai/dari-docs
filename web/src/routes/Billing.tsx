import { useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { Input } from "@/components/ui/input";
import {
  createCheckout,
  getBalance,
  getBillingConfig,
  getRunConfig,
  type BillingConfig,
  type RunConfig,
} from "@/lib/billing";
import { formatCents } from "@/lib/utils";

const fallbackBillingConfig: BillingConfig = {
  min_checkout_cents: 500,
  default_checkout_cents: 500,
  max_checkout_cents: 50000,
};

export default function Billing() {
  const [balance, setBalance] = useState<number | null>(null);
  const [config, setConfig] = useState<RunConfig | null>(null);
  const [billingConfig, setBillingConfig] = useState<BillingConfig>(fallbackBillingConfig);
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
  const invalid =
    amountCents < billingConfig.min_checkout_cents || amountCents > billingConfig.max_checkout_cents;
  const freeCreditCents = config?.free_credit_cents ?? 500;
  const balanceCents = balance ?? 0;

  const startCheckout = async () => {
    if (invalid) {
      setCheckoutError(
        `Enter an amount between ${formatCents(billingConfig.min_checkout_cents)} and ${formatCents(billingConfig.max_checkout_cents)}.`
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
        <h1 className="text-xl font-medium">Billing</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Credit balance for managed Dari Docs runs.
        </p>
      </div>

      {error && (
        <div className="mb-6 border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive-foreground">
          {error}
        </div>
      )}

      {loading ? (
        <div className="text-sm text-muted-foreground">loading billing...</div>
      ) : (
        <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_360px]">
          <Card>
            <CardHeader>
              <CardTitle>Credit balance</CardTitle>
              <CardDescription>
                New accounts start with {formatCents(freeCreditCents)} in credits.
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div>
                  <div className="text-3xl font-medium tracking-tight">
                    {formatCents(balanceCents)}
                  </div>
                  <div className="mt-1 text-xs text-muted-foreground">available for managed runs</div>
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setCheckoutError(null);
                    setBuyOpen(true);
                  }}
                >
                  Purchase credits
                </Button>
              </div>
            </CardContent>
          </Card>

          <section className="border border-border bg-card p-6">
            <div className="text-sm font-medium">Managed limits</div>
            <dl className="mt-4 flex flex-col gap-3 text-sm">
              <Row label="Active runs" value={`${config?.max_active_runs_per_user ?? "-"} per account`} />
              <Row label="Tasks per run" value={`${config?.max_tasks_per_run ?? "-"} max`} />
              <Row label="Tester reserve" value={formatCents(config?.tester_session_reserve_cents ?? 0)} />
              <Row label="Editor reserve" value={formatCents(config?.editor_session_reserve_cents ?? 0)} />
            </dl>
          </section>
        </div>
      )}

      <ConfirmDialog
        open={buyOpen}
        onOpenChange={setBuyOpen}
        title="Buy Dari Docs credits"
        confirmLabel="Checkout"
        confirming={checkingOut}
        onConfirm={startCheckout}
        error={checkoutError}
        description={
          <span className="flex flex-col gap-3">
            <span>Credits are added after Stripe confirms payment.</span>
            <span className="flex flex-col gap-2">
              <label className="text-xs uppercase tracking-widest text-muted-foreground">
                Credits to buy
              </label>
              <Input
                type="number"
                min={String(billingConfig.min_checkout_cents / 100)}
                max={String(billingConfig.max_checkout_cents / 100)}
                step="0.01"
                value={amount}
                onChange={(event) => setAmount(event.target.value)}
              />
              <span className="text-xs">
                Minimum {formatCents(billingConfig.min_checkout_cents)}.
              </span>
            </span>
          </span>
        }
      />
    </div>
  );
}

function centsToDollars(cents: number): string {
  return (cents / 100).toFixed(2);
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4 border-t border-border pt-3">
      <dt className="text-muted-foreground">{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}
