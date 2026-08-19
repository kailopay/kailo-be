package stellar

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/service/settlement"
	"github.com/stellar/go-stellar-sdk/keypair"
)

func TestBuildCreatesSignedNativeXLMTransaction(t *testing.T) {
	const secret = "SBPQUZ6G4FZNWFHKUWC5BEYWF6R52E3SEP7R3GWYSM2XTKGF5LNTWW4R"
	source := keypair.MustParseFull(secret).Address()
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
	built, err := client.Build(context.Background(), settlement.Transfer{OrderID: "order-1", Source: source, Destination: destination, Amount: 10_000_000})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if built.Hash == "" || built.Envelope == "" {
		t.Fatalf("built = %+v", built)
	}
}
