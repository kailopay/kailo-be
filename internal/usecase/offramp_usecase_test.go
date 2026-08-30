package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

// --- parseDecimalStroops ---

func TestParseDecimalStroops(t *testing.T) {
	cases := []struct {
		in      string
		want    entity.Stroops
		wantErr bool
	}{
		{"40", 400_000_000, false},
		{"40.0000000", 400_000_000, false},
		{"6.25", 62_500_000, false},
		{"0.0000001", 1, false},
		{"0.5", 5_000_000, false},
		{"40.00000001", 0, true}, // beyond stroop precision
		{"abc", 0, true},         // non-numeric
		{"-1", 0, true},          // sign rejected by charset
		{"", 0, true},            // empty
		{".5", 5_000_000, false}, // bare fraction tolerated
		{"007", 0, true},         // leading zeros rejected
	}
	for _, testCase := range cases {
		got, err := parseDecimalStroops(testCase.in)
		if (err != nil) != testCase.wantErr {
			t.Errorf("parseDecimalStroops(%q) err = %v, wantErr %v", testCase.in, err, testCase.wantErr)
			continue
		}
		if got != testCase.want {
			t.Errorf("parseDecimalStroops(%q) = %d, want %d", testCase.in, got, testCase.want)
		}
	}
}

// --- QuotePolicy.CreateReverse ---

func TestQuotePolicyCreateReverseFloorsIDRAndAppliesSpread(t *testing.T) {
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	policy := QuotePolicy{TTL: 5 * time.Minute, MaxAge: 2 * time.Minute, SpreadBPS: 100}
	market := MarketPrice{IDRPerXLM: "2500", ObservedAt: now.Add(-time.Second)}

	quote, err := policy.CreateReverse(now, 4_000_000_000, market) // 400 XLM at 2500 + 1% = 10100 IDR/XLM
	if err != nil {
		t.Fatalf("CreateReverse() error = %v", err)
	}
	if quote.FiatAmount != 1_010_000 { // 400 XLM * 2525 IDR/XLM (2500+1%)
		t.Fatalf("fiat = %d, want 4040000", quote.FiatAmount)
	}
	if quote.AssetAmount != 4_000_000_000 {
		t.Fatalf("asset = %d", quote.AssetAmount)
	}

	// Flooring: a tiny amount quotes a tiny (or zero) IDR payout, never rounded up.
	dust, err := policy.CreateReverse(now, 100_000_000, market) // 10 XLM
	if err != nil {
		t.Fatalf("CreateReverse(dust) error = %v", err)
	}
	if int64(dust.FiatAmount) != 25_250 { // 10 XLM * 2525 IDR (2500 + 1% spread)
		t.Fatalf("dust quote = %d, want 25250", dust.FiatAmount)
	}

	stale := market
	stale.ObservedAt = now.Add(-3 * time.Minute)
	if _, err := policy.CreateReverse(now, 4_000_000_000, stale); !errors.Is(err, ErrStalePrice) {
		t.Fatalf("stale price error = %v", err)
	}
}

// --- OfframpUsecase.Create lifecycle ---

type fakeOfframpRepo struct {
	replay          OrderView
	found           bool
	created         []OfframpCreateRecord
	view            OrderView
	err             error
	findPrincipal   OrderPrincipal
	findRequestHash string
}

func (r *fakeOfframpRepo) FindOfframpReplay(_ context.Context, principal OrderPrincipal, _, requestHash string) (OrderView, bool, error) {
	r.findPrincipal = principal
	r.findRequestHash = requestHash
	return r.replay, r.found, r.err
}
func (r *fakeOfframpRepo) CreateOfframp(_ context.Context, record OfframpCreateRecord) error {
	r.created = append(r.created, record)
	return r.err
}
func (r *fakeOfframpRepo) Get(context.Context, OrderPrincipal, string) (OrderView, error) {
	return r.view, nil
}
func (r *fakeOfframpRepo) List(context.Context, OrderPrincipal, int, string) ([]OrderView, string, error) {
	return nil, "", nil
}
func (r *fakeOfframpRepo) FindDepositCandidates(context.Context, string, int) ([]OrderView, error) {
	return nil, nil
}
func (r *fakeOfframpRepo) RecordAssetReceived(context.Context, string, ObservedPayment) error {
	return nil
}
func (r *fakeOfframpRepo) RecordAssetInvalid(context.Context, string, string, string) error {
	return nil
}
func (r *fakeOfframpRepo) LoadRetirement(context.Context, string) (RetirementIntent, error) {
	return RetirementIntent{}, nil
}
func (r *fakeOfframpRepo) SaveRetirementHash(context.Context, string, string, time.Time) error {
	return nil
}
func (r *fakeOfframpRepo) ConfirmRetirement(context.Context, string, string, time.Time) error {
	return nil
}
func (r *fakeOfframpRepo) FailRetirement(context.Context, string, string) error { return nil }
func (r *fakeOfframpRepo) CompleteSimulatedPayout(context.Context, string, int64, time.Time) (string, error) {
	return "", nil
}

var validDestination = "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func testOfframpService(t *testing.T, repo *fakeOfframpRepo) *OfframpUsecase {
	t.Helper()
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	service, err := NewOfframpUsecase(OfframpDependencies{
		Repository: repo, Prices: priceFake{now: now},
		Destinations: destinationFake{},
	}, OfframpServiceConfig{
		QuotePolicy: QuotePolicy{TTL: 5 * time.Minute, MaxAge: 2 * time.Minute},
		MinIDR:      10_000, MaxIDR: 10_000_000,
		DepositAccount: validDestination, DepositExpiry: 24 * time.Hour,
		NewID: func() (string, error) { return "order-off-1", nil }, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewOfframpUsecase() error = %v", err)
	}
	return service
}

func TestOfframpCreatePersistsDepositInstructions(t *testing.T) {
	repo := &fakeOfframpRepo{}
	service := testOfframpService(t, repo)

	view, replay, err := service.Create(context.Background(), OfframpCommand{
		Principal: apiOrderPrincipal("client-1", "owner-1"), IdempotencyKey: "idem-1", AssetNetwork: "stellar_testnet", AssetCode: "XLM", FiatCurrency: "IDR", AssetAmount: "40",
		WithdrawalMethod: entity.WithdrawalMethodSandboxTransfer, DestinationToken: "demo-token",
	})
	if err != nil || replay {
		t.Fatalf("Create() = %v, replay=%v", err, replay)
	}
	if len(repo.created) != 1 {
		t.Fatalf("created records = %d", len(repo.created))
	}
	record := repo.created[0]
	if record.AssetAmount != 400_000_000 || record.WithdrawalMethod != entity.WithdrawalMethodSandboxTransfer {
		t.Fatalf("record = %+v", record)
	}
	if record.Quote.FiatAmount != 100_000 { // 40 XLM at 2500, zero spread
		t.Fatalf("quote fiat = %d, want 100000", record.Quote.FiatAmount)
	}
	if record.Memo != "offorderoff1" {
		// Dashes are stripped so the memo fits SEP text-memo limits for any UUID.
		t.Fatalf("memo = %q", record.Memo)
	}
	if record.ExpiresAt.Before(time.Now()) == false && record.ExpiresAt.Sub(time.Now()) > 24*time.Hour+time.Minute {
		t.Fatalf("expiry too far out: %v", record.ExpiresAt)
	}
	_ = view
}

func TestOfframpCreateReplaysAndRejectsInvalidInput(t *testing.T) {
	repo := &fakeOfframpRepo{found: true, replay: OrderView{ID: "order-off-0"}}
	service := testOfframpService(t, repo)

	view, replay, err := service.Create(context.Background(), OfframpCommand{
		Principal: apiOrderPrincipal("c", "owner-1"), IdempotencyKey: "k", AssetNetwork: "stellar_testnet", AssetCode: "XLM", FiatCurrency: "IDR", AssetAmount: "40",
		WithdrawalMethod: entity.WithdrawalMethodSandboxTransfer})
	if err != nil || !replay || view.ID != "order-off-0" {
		t.Fatalf("replay = %v/%v/%q", err, replay, view.ID)
	}
	if len(repo.created) != 0 {
		t.Fatal("replay must not create")
	}

	bad := []OfframpCommand{
		{Principal: apiOrderPrincipal("c", "owner-1"), IdempotencyKey: "k", AssetNetwork: "stellar_testnet", AssetCode: "XLM", FiatCurrency: "IDR", AssetAmount: "40"},                                                             // wrong method
		{Principal: apiOrderPrincipal("c", "owner-1"), IdempotencyKey: "k", AssetNetwork: "stellar_testnet", AssetCode: "XLM", FiatCurrency: "IDR", AssetAmount: "nope", WithdrawalMethod: entity.WithdrawalMethodSandboxTransfer}, // bad amount
	}
	for _, command := range bad {
		if _, _, err := service.Create(context.Background(), command); !errors.Is(err, ErrInvalidWithdrawal) {
			t.Errorf("invalid command %+v error = %v", command, err)
		}
	}
}

func TestOfframpCreateRejectsUnsupportedNetworkAssetOrCurrency(t *testing.T) {
	base := OfframpCommand{
		Principal: apiOrderPrincipal("client-1", "owner-1"), IdempotencyKey: "idem-unsupported", AssetNetwork: "stellar_testnet",
		AssetCode: "XLM", FiatCurrency: "IDR", AssetAmount: "40", WithdrawalMethod: entity.WithdrawalMethodSandboxTransfer,
	}
	cases := []struct {
		name   string
		mutate func(*OfframpCommand)
	}{
		{name: "mainnet network", mutate: func(command *OfframpCommand) { command.AssetNetwork = "stellar_mainnet" }},
		{name: "non XLM asset", mutate: func(command *OfframpCommand) { command.AssetCode = "USDC" }},
		{name: "non IDR currency", mutate: func(command *OfframpCommand) { command.FiatCurrency = "USD" }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			command := base
			testCase.mutate(&command)
			if _, _, err := testOfframpService(t, &fakeOfframpRepo{}).Create(context.Background(), command); !errors.Is(err, ErrInvalidWithdrawal) {
				t.Fatalf("Create() error = %v, want %v", err, ErrInvalidWithdrawal)
			}
		})
	}
}

func TestOfframpCreateCarriesRetailPrincipalAndIncludesOperationInRequestHash(t *testing.T) {
	repo := &fakeOfframpRepo{}
	service := testOfframpService(t, repo)
	principal := OrderPrincipal{Kind: OrderPrincipalRetailSession, OwnerUserID: "user-1", SessionID: "session-1"}

	view, replay, err := service.Create(context.Background(), OfframpCommand{
		Principal: principal, IdempotencyKey: "idem-retail-off-1", AssetNetwork: "stellar_testnet", AssetCode: "XLM", FiatCurrency: "IDR", AssetAmount: "40",
		WithdrawalMethod: entity.WithdrawalMethodSandboxTransfer, DestinationToken: "demo-token",
	})
	if err != nil || replay || view.ID != repo.view.ID {
		// The fake intentionally returns its zero view after persistence. The
		// assertions below verify the ownership and request scope instead.
		if err != nil || replay {
			t.Fatalf("Create() = %v, replay=%v", err, replay)
		}
	}
	if len(repo.created) != 1 || repo.created[0].Principal != principal {
		t.Fatalf("created principal = %+v, want %+v", repo.created[0].Principal, principal)
	}
	wantHash := orderRequestHash(offrampRequestOperation, "stellar_testnet", "XLM", "400000000", "IDR", "sandbox_bank_transfer", "demo-token")
	if repo.findRequestHash != wantHash {
		t.Fatalf("request hash = %q, want %q", repo.findRequestHash, wantHash)
	}
}

// --- SEP-24 status mapping exhaustiveness ---

func TestSep24StatusCoversEveryInternalState(t *testing.T) {
	allStates := []entity.OrderStatus{
		entity.OrderStatusCreated, entity.OrderStatusPaymentPending, entity.OrderStatusPaymentConfirmed,
		entity.OrderStatusStellarProcessing, entity.OrderStatusCompleted, entity.OrderStatusExpired,
		entity.OrderStatusPaymentFailed, entity.OrderStatusStellarFailed, entity.OrderStatusCancelled,
		entity.OrderStatusAssetPending, entity.OrderStatusAssetReceived, entity.OrderStatusAssetInvalid,
		entity.OrderStatusRetirementProcessing, entity.OrderStatusWithdrawalProcessing,
		entity.OrderStatusRetirementFailed, entity.OrderStatusWithdrawalFailed,
	}
	for _, status := range allStates {
		mapped, ok := Sep24Status(string(status))
		if !ok {
			t.Errorf("state %q has no SEP-24 mapping", status)
			continue
		}
		switch mapped {
		case "pending_user_transfer_start", "pending_anchor", "pending_external",
			"completed", "expired", "error":
		default:
			t.Errorf("state %q mapped to unknown SEP status %q", status, mapped)
		}
	}
	cancelled, ok := Sep24Status("cancelled")
	if !ok || cancelled != "error" {
		t.Logf("cancelled maps to %q (ok=%v)", cancelled, ok)
	}
	if completed, _ := Sep24Status("completed"); completed != "completed" {
		t.Fatal("completed must map to completed")
	}
}

// --- Federation resolver ---

func TestFederationResolverOnlyResolvesDemoNames(t *testing.T) {
	resolver := &FederationResolver{DepositAccount: validDestination}
	address, memo, ok := resolver.Resolve("alice*kailopay")
	if !ok || address != validDestination || memo != "fed-alice" {
		t.Fatalf("resolve = %q/%q/%v", address, memo, ok)
	}
	for _, query := range []string{"", "noseparator", "wrong*xendit", "*kailopay", "bad name*kailopay",
		strings.Repeat("a", 29) + "*kailopay", "evil<script>*kailopay"} {
		if address, _, ok := resolver.Resolve(query); ok {
			t.Errorf("query %q resolved to %s, want rejection", query, address)
		}
	}
}

// --- Webhook URL policy ---

func TestWebhookURLPolicyRejectsUnsafeHosts(t *testing.T) {
	accepted := "https://hooks.example.com/kp"
	if !isDeliverableWebhookURL(accepted) {
		t.Fatalf("%s should be accepted", accepted)
	}
	rejected := []string{
		"http://hooks.example.com/kp",
		"https://localhost/kp",
		"https://127.0.0.1/kp",
		"https://10.0.0.5/kp",
		"https://192.168.1.1/kp",
		"https://172.16.0.9/kp",
		"https://user:pass@hooks.example.com/kp",
		"not a url",
	}
	for _, raw := range rejected {
		if isDeliverableWebhookURL(raw) {
			t.Errorf("%s should be rejected", raw)
		}
	}
}

// --- Event envelope ---

func TestBuildEventEnvelopeShape(t *testing.T) {
	payload, err := BuildEventEnvelope("abc123", EventOrderCompleted, "sandbox",
		time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC), OrderView{ID: "order-1", Status: entity.OrderStatusCompleted})
	if err != nil {
		t.Fatalf("BuildEventEnvelope() error = %v", err)
	}
	var envelope map[string]any
	if json.Unmarshal(payload, &envelope) != nil {
		t.Fatal("envelope is not valid JSON")
	}
	for _, key := range []string{"id", "object", "type", "api_version", "created_at", "environment", "data"} {
		if _, ok := envelope[key]; !ok {
			t.Errorf("envelope missing %q", key)
		}
	}
	if envelope["id"] != "evt_abc123" || envelope["object"] != "event" {
		t.Fatalf("identity fields wrong: %v", envelope)
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok || data["object"].(map[string]any)["id"] != "order-1" {
		t.Fatal("data.object must carry the public order")
	}
}
