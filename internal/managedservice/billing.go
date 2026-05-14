package managedservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
)

func (s *Server) handleBalance(w http.ResponseWriter, r *http.Request, u user) {
	s.handleMe(w, r, u)
}

func (s *Server) handleRunConfig(w http.ResponseWriter, r *http.Request, u user) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"free_credit_cents":             s.cfg.FreeGrantCents,
		"tester_session_reserve_cents":  s.cfg.TesterReserveCents,
		"editor_session_reserve_cents":  s.cfg.EditorReserveCents,
		"service_fee_cents":             s.cfg.ServiceFeeCents,
		"max_tasks_per_run":             s.cfg.MaxTasksPerRun,
		"max_task_bytes":                s.cfg.MaxTaskBytes,
		"max_active_runs_per_user":      s.maxActiveRunsPerUser(),
		"max_bundle_bytes":              s.cfg.MaxBundleBytes,
		"bundle_max_uncompressed_bytes": s.cfg.BundleMaxUncompressedBytes,
		"bundle_max_file_bytes":         s.cfg.BundleMaxFileBytes,
	})
}

func (s *Server) handleCheckout(w http.ResponseWriter, r *http.Request, u user) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.cfg.StripeSecretKey == "" {
		writeError(w, http.StatusNotImplemented, "STRIPE_SECRET_KEY is not configured")
		return
	}
	var req struct {
		AmountCents int64 `json:"amount_cents"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.AmountCents < stripeCheckoutMinCents || req.AmountCents > stripeCheckoutMaxCents {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("amount_cents must be between %d and %d", stripeCheckoutMinCents, stripeCheckoutMaxCents))
		return
	}
	checkout, err := s.createStripeCheckout(r.Context(), u, req.AmountCents)
	if err != nil {
		writeLoggedError(w, http.StatusBadGateway, "could not create checkout session", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"checkout_session_id": checkout.ID, "checkout_url": checkout.URL})
}

func (s *Server) handleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeLoggedError(w, http.StatusBadRequest, "invalid webhook request", err)
		return
	}
	if s.cfg.StripeWebhookSecret == "" {
		writeError(w, http.StatusServiceUnavailable, "STRIPE_WEBHOOK_SECRET is not configured")
		return
	}
	event, err := webhook.ConstructEventWithOptions(body, r.Header.Get("Stripe-Signature"), s.cfg.StripeWebhookSecret, webhook.ConstructEventOptions{
		Tolerance:                s.cfg.StripeWebhookTolerance,
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		writeLoggedError(w, http.StatusUnauthorized, "invalid Stripe signature", err)
		return
	}
	if event.Type != "checkout.session.completed" {
		writeJSON(w, http.StatusOK, map[string]any{"handled": false})
		return
	}
	var session stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
		writeLoggedError(w, http.StatusBadRequest, "invalid Stripe webhook payload", err)
		return
	}
	credited, err := s.creditPaidStripeCheckout(r.Context(), session)
	if err != nil {
		if errors.Is(err, errStripeCheckoutIgnored) {
			writeJSON(w, http.StatusOK, map[string]any{"handled": true, "credited": false})
			return
		}
		status := http.StatusInternalServerError
		if errors.Is(err, errStripeCheckoutInvalid) {
			status = http.StatusBadRequest
		}
		if errors.Is(err, errStripeCheckoutInvalid) {
			writeLoggedError(w, status, "invalid Stripe checkout session", err)
		} else {
			writeLoggedError(w, status, "could not process Stripe webhook", err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"handled": true, "credited": credited})
}

func (s *Server) handleBillingSuccess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeHTML(w, http.StatusOK, "Dari Docs credits purchased", "Your payment completed. Stripe will apply the credits to your Dari Docs account shortly.")
}

func (s *Server) handleBillingCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeHTML(w, http.StatusOK, "Dari Docs checkout canceled", "No payment was taken. You can return to the CLI and start checkout again.")
}

func (s *Server) balanceCents(ctx context.Context, userID string) (int64, error) {
	return balanceCentsQuery(ctx, s.db, userID)
}

type balanceQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func balanceCentsTx(ctx context.Context, tx pgx.Tx, userID string) (int64, error) {
	return balanceCentsQuery(ctx, tx, userID)
}

func balanceCentsQuery(ctx context.Context, q balanceQuerier, userID string) (int64, error) {
	var cents int64
	err := q.QueryRow(ctx, `SELECT coalesce(sum(amount_cents),0) FROM credit_ledger WHERE user_id=$1`, userID).Scan(&cents)
	return cents, err
}

type stripeCheckout struct {
	ID  string
	URL string
}

var createStripeCheckoutSession = func(ctx context.Context, secret string, httpClient *http.Client, params *stripe.CheckoutSessionCreateParams) (*stripe.CheckoutSession, error) {
	if httpClient == nil {
		httpClient = defaultOutboundHTTPClient
	}
	backends := stripe.NewBackendsWithConfig(&stripe.BackendConfig{HTTPClient: httpClient})
	return stripe.NewClient(secret, stripe.WithBackends(backends)).V1CheckoutSessions.Create(ctx, params)
}

func (s *Server) createStripeCheckout(ctx context.Context, u user, amountCents int64) (stripeCheckout, error) {
	metadata := map[string]string{
		"billing_kind": "dari_docs_credit_purchase",
		"user_id":      u.ID,
		"amount_cents": strconv.FormatInt(amountCents, 10),
	}
	params := &stripe.CheckoutSessionCreateParams{
		Mode:               stripe.String("payment"),
		SuccessURL:         stripe.String(strings.TrimRight(s.cfg.PublicBaseURL, "/") + "/billing/success?session_id={CHECKOUT_SESSION_ID}"),
		CancelURL:          stripe.String(strings.TrimRight(s.cfg.PublicBaseURL, "/") + "/billing/cancel"),
		CustomerEmail:      stripe.String(u.Email),
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
		LineItems: []*stripe.CheckoutSessionCreateLineItemParams{{
			Quantity: stripe.Int64(1),
			PriceData: &stripe.CheckoutSessionCreateLineItemPriceDataParams{
				Currency:   stripe.String("usd"),
				UnitAmount: stripe.Int64(amountCents),
				ProductData: &stripe.CheckoutSessionCreateLineItemPriceDataProductDataParams{
					Name: stripe.String("dari-docs credits"),
				},
			},
		}},
		Metadata: metadata,
	}
	session, err := createStripeCheckoutSession(ctx, s.cfg.StripeSecretKey, s.outboundHTTPClient(), params)
	if err != nil {
		return stripeCheckout{}, err
	}
	if session.ID == "" || session.URL == "" {
		return stripeCheckout{}, errors.New("stripe checkout response missing id or url")
	}
	_, err = s.db.Exec(ctx, `
INSERT INTO stripe_checkout_sessions (id, user_id, amount_cents, currency, status)
VALUES ($1, $2, $3, 'usd', 'created')
`, session.ID, u.ID, amountCents)
	if err != nil {
		return stripeCheckout{}, err
	}
	return stripeCheckout{ID: session.ID, URL: session.URL}, nil
}

var (
	errStripeCheckoutIgnored = errors.New("stripe checkout ignored")
	errStripeCheckoutInvalid = errors.New("invalid stripe checkout")
)

type persistedStripeCheckout struct {
	UserID      string
	AmountCents int64
	Currency    string
}

func (s *Server) creditPaidStripeCheckout(ctx context.Context, session stripe.CheckoutSession) (bool, error) {
	if session.PaymentStatus != stripe.CheckoutSessionPaymentStatusPaid {
		return false, errStripeCheckoutIgnored
	}
	if session.Metadata["billing_kind"] != "dari_docs_credit_purchase" {
		return false, errStripeCheckoutIgnored
	}
	if session.ID == "" {
		return false, fmt.Errorf("%w: missing session id", errStripeCheckoutInvalid)
	}
	userID := strings.TrimSpace(session.Metadata["user_id"])
	amountCents, err := strconv.ParseInt(strings.TrimSpace(session.Metadata["amount_cents"]), 10, 64)
	if err != nil || userID == "" || amountCents <= 0 {
		return false, fmt.Errorf("%w: missing user_id or amount_cents metadata", errStripeCheckoutInvalid)
	}
	if session.AmountTotal != amountCents {
		return false, fmt.Errorf("%w: amount_total mismatch", errStripeCheckoutInvalid)
	}
	currency := strings.ToLower(string(session.Currency))
	if currency == "" {
		currency = "usd"
	}
	if currency != "usd" {
		return false, fmt.Errorf("%w: unsupported currency %q", errStripeCheckoutInvalid, currency)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var persisted persistedStripeCheckout
	err = tx.QueryRow(ctx, `
SELECT user_id, amount_cents, currency
FROM stripe_checkout_sessions
WHERE id=$1
FOR UPDATE
`, session.ID).Scan(&persisted.UserID, &persisted.AmountCents, &persisted.Currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, errStripeCheckoutIgnored
	}
	if err != nil {
		return false, err
	}
	if persisted.UserID != userID || persisted.AmountCents != amountCents || strings.ToLower(persisted.Currency) != currency {
		return false, fmt.Errorf("%w: persisted checkout mismatch", errStripeCheckoutInvalid)
	}
	tag, err := tx.Exec(ctx, `
INSERT INTO credit_ledger (id, user_id, amount_cents, kind, source_id)
VALUES ($1, $2, $3, 'stripe_checkout', $4)
ON CONFLICT (source_id) DO NOTHING
`, "led_"+randomToken(18), persisted.UserID, persisted.AmountCents, session.ID)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `
UPDATE stripe_checkout_sessions
SET status='credited',
    completed_at=coalesce(completed_at, now()),
    credited_at=coalesce(credited_at, now())
WHERE id=$1
`, session.ID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, tx.Commit(ctx)
}

func usdStringToCentsCeil(v string) int64 {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	whole, frac, ok := strings.Cut(v, ".")
	if !ok {
		d, _ := strconv.ParseInt(whole, 10, 64)
		return d * 100
	}
	d, _ := strconv.ParseInt(whole, 10, 64)
	frac = strings.TrimRight(frac, "0")
	centsText := frac
	if len(centsText) > 2 {
		centsText = centsText[:2]
	}
	for len(centsText) < 2 {
		centsText += "0"
	}
	cents, _ := strconv.ParseInt(centsText, 10, 64)
	if len(frac) > 2 {
		cents++
	}
	return d*100 + cents
}
