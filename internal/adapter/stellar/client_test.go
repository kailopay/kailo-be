package stellar

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/stellar/go-stellar-sdk/keypair"
)

func TestBuildCreatesSignedNativeXLMTransaction(t *testing.T) {
	var rawSeed [32]byte
	copy(rawSeed[:], []byte("kailopay-week-one-test-seed"))
	sourceKey, err := keypair.FromRawSeed(rawSeed)
	if err != nil {
		t.Fatal(err)
	}
	secret := sourceKey.Seed()
	source := sourceKey.Address()
	destination := keypair.MustRandom().Address()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts/"+source {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"sequence":"12345","balances":[{"asset_type":"native","balance":"100.0000000","selling_liabilities":"0.0000000"}]}`)
	}))
	defer server.Close()
	client, err := New(Config{HorizonURL: server.URL, NetworkPassphrase: "Test SDF Network ; September 2015",
		TreasurySecret: secret, HTTPClient: server.Client(), TransactionTimeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	built, err := client.Build(context.Background(), usecase.Transfer{OrderID: "order-1", Source: source, Destination: destination, Amount: 10_000_000})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if built.Hash == "" || built.Envelope == "" {
		t.Fatalf("built = %+v", built)
	}
}

func newSigningTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	var rawSeed [32]byte
	copy(rawSeed[:], []byte("kailopay-week-one-test-seed"))
	sourceKey, err := keypair.FromRawSeed(rawSeed)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := New(Config{HorizonURL: server.URL, NetworkPassphrase: "Test SDF Network ; September 2015",
		TreasurySecret: sourceKey.Seed(), HTTPClient: server.Client(), TransactionTimeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestSubmitClassifiesHorizonRejections(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		wantResult usecase.SubmissionResult
	}{
		{"sequence conflict is retryable", http.StatusBadRequest,
			`{"error":{"extras":{"result_codes":{"transaction":"tx_bad_seq"}}}}`, usecase.SubmissionRetryable},
		{"other rejection is permanent", http.StatusBadRequest,
			`{"error":{"extras":{"result_codes":{"transaction":"tx_no_account"}}}}`, usecase.SubmissionPermanent},
		{"server error is unknown", http.StatusBadGateway, `{}`, usecase.SubmissionUnknown},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			client := newSigningTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(testCase.status)
				_, _ = fmt.Fprint(w, testCase.body)
			})
			submission, err := client.Submit(context.Background(), usecase.BuiltTransaction{Hash: "hash-1", Envelope: "envelope"})
			if err != nil {
				t.Fatalf("Submit() error = %v", err)
			}
			if submission.Result != testCase.wantResult {
				t.Fatalf("result = %q, want %q", submission.Result, testCase.wantResult)
			}
		})
	}
}

func TestFindByHashVerifiesReturnedTransaction(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantResult usecase.SubmissionResult
	}{
		{"confirmed success", `{"hash":"hash-1","created_at":"2026-08-22T00:00:00Z","successful":true}`, usecase.SubmissionConfirmed},
		{"failed transaction", `{"hash":"hash-1","successful":false}`, usecase.SubmissionPermanent},
		{"hash mismatch stays unknown", `{"hash":"hash-other","successful":true}`, usecase.SubmissionUnknown},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			client := newSigningTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = fmt.Fprint(w, testCase.body)
			})
			submission, err := client.FindByHash(context.Background(), "hash-1")
			if err != nil {
				t.Fatalf("FindByHash() error = %v", err)
			}
			if submission.Result != testCase.wantResult {
				t.Fatalf("result = %q, want %q", submission.Result, testCase.wantResult)
			}
		})
	}
}
