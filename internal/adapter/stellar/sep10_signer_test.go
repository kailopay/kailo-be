package stellar

import (
	"testing"
	"time"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/txnbuild"
)

const sep10TestNetwork = "Test SDF Network ; September 2015"

func sep10TestKey(t *testing.T, seed string) *keypair.Full {
	t.Helper()
	var raw [32]byte
	copy(raw[:], []byte(seed))
	key, err := keypair.FromRawSeed(raw)
	if err != nil {
		t.Fatalf("FromRawSeed() error = %v", err)
	}
	return key
}

func newSEP10SignerForTest(t *testing.T, network, homeDomain string) *SEP10Signer {
	t.Helper()
	server := sep10TestKey(t, "sep10-server-test-seed")
	signer, err := NewSEP10Signer(SEP10SignerConfig{
		ServerSecret: server.Seed(), Network: network, WebAuthDomain: "anchor.example.com",
		HomeDomain: homeDomain, Timebound: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewSEP10Signer() error = %v", err)
	}
	return signer
}

func signedSEP10Challenge(t *testing.T, signer *SEP10Signer, client *keypair.Full) string {
	t.Helper()
	challenge, _, err := signer.BuildChallenge(client.Address())
	if err != nil {
		t.Fatalf("BuildChallenge() error = %v", err)
	}
	return signSEP10Challenge(t, challenge, client)
}

func signSEP10Challenge(t *testing.T, challenge string, client *keypair.Full) string {
	t.Helper()
	generic, err := txnbuild.TransactionFromXDR(challenge)
	if err != nil {
		t.Fatalf("TransactionFromXDR() error = %v", err)
	}
	tx, ok := generic.Transaction()
	if !ok {
		t.Fatal("challenge is not a classic transaction")
	}
	signed, err := tx.Sign(sep10TestNetwork, client)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	encoded, err := signed.Base64()
	if err != nil {
		t.Fatalf("Base64() error = %v", err)
	}
	return encoded
}

func TestSEP10SignerBuildsAndVerifiesClassicWalletChallenge(t *testing.T) {
	server := sep10TestKey(t, "sep10-server-test-seed")
	client := sep10TestKey(t, "sep10-client-test-seed")
	signer := newSEP10SignerForTest(t, sep10TestNetwork, "anchor.example.com")
	challenge, hash, err := signer.BuildChallenge(client.Address())
	if err != nil {
		t.Fatalf("BuildChallenge() error = %v", err)
	}
	if challenge == "" || len(hash) != 32 || signer.ServerAccount() != server.Address() {
		t.Fatalf("challenge/hash/server = %q/%d/%q", challenge, len(hash), signer.ServerAccount())
	}
	signed := signSEP10Challenge(t, challenge, client)
	account, verifiedHash, err := signer.VerifyChallenge(signed)
	if err != nil {
		t.Fatalf("VerifyChallenge() error = %v", err)
	}
	if account != client.Address() || string(verifiedHash) != string(hash) {
		t.Fatalf("verified account/hash = %q/%x, want %q/%x", account, verifiedHash, client.Address(), hash)
	}
}

func TestSEP10SignerRejectsWrongNetworkHomeDomainAndSignature(t *testing.T) {
	client := sep10TestKey(t, "sep10-client-test-seed")
	signer := newSEP10SignerForTest(t, sep10TestNetwork, "anchor.example.com")
	signed := signedSEP10Challenge(t, signer, client)

	wrongNetwork := newSEP10SignerForTest(t, "Public Global Stellar Network ; September 2015", "anchor.example.com")
	if _, _, err := wrongNetwork.VerifyChallenge(signed); err == nil {
		t.Fatal("VerifyChallenge() error = nil for wrong network")
	}
	wrongDomain := newSEP10SignerForTest(t, sep10TestNetwork, "other.example.com")
	if _, _, err := wrongDomain.VerifyChallenge(signed); err == nil {
		t.Fatal("VerifyChallenge() error = nil for wrong home domain")
	}
	wrongClient := sep10TestKey(t, "sep10-other-client-seed")
	challenge, _, err := signer.BuildChallenge(client.Address())
	if err != nil {
		t.Fatalf("BuildChallenge() error = %v", err)
	}
	invalid := signSEP10Challenge(t, challenge, wrongClient)
	if _, _, err := signer.VerifyChallenge(invalid); err == nil {
		t.Fatal("VerifyChallenge() error = nil for invalid wallet signature")
	}
}
