package stellar

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/txnbuild"
)

type SEP10SignerConfig struct {
	ServerSecret  string
	Network       string
	WebAuthDomain string
	HomeDomain    string
	Timebound     time.Duration
}

type SEP10Signer struct {
	config        SEP10SignerConfig
	server        *keypair.Full
	serverAccount string
}

func NewSEP10Signer(config SEP10SignerConfig) (*SEP10Signer, error) {
	if strings.TrimSpace(config.ServerSecret) == "" || strings.TrimSpace(config.Network) == "" ||
		strings.TrimSpace(config.WebAuthDomain) == "" || strings.TrimSpace(config.HomeDomain) == "" || config.Timebound <= 0 {
		return nil, errors.New("valid SEP-10 signer configuration is required")
	}
	parsed, err := keypair.Parse(config.ServerSecret)
	if err != nil {
		return nil, fmt.Errorf("parsing SEP-10 server secret: %w", err)
	}
	server, ok := parsed.(*keypair.Full)
	if !ok {
		return nil, errors.New("SEP-10 server secret must be a signing key")
	}
	return &SEP10Signer{config: config, server: server, serverAccount: server.Address()}, nil
}

func (s *SEP10Signer) ServerAccount() string { return s.serverAccount }

func (s *SEP10Signer) ValidateAccount(account string) error {
	account = strings.TrimSpace(account)
	if account == "" || !strings.HasPrefix(account, "G") {
		return errors.New("SEP-10 requires a classic Stellar account")
	}
	if _, err := keypair.ParseAddress(account); err != nil {
		return fmt.Errorf("invalid Stellar account: %w", err)
	}
	return nil
}

func (s *SEP10Signer) BuildChallenge(account string) (string, []byte, error) {
	if err := s.ValidateAccount(account); err != nil {
		return "", nil, err
	}
	transaction, err := txnbuild.BuildChallengeTx(
		s.server.Seed(), account, s.config.WebAuthDomain, s.config.HomeDomain,
		s.config.Network, s.config.Timebound, nil,
	)
	if err != nil {
		return "", nil, fmt.Errorf("building SEP-10 challenge transaction: %w", err)
	}
	hash, err := sep10ChallengeHash(transaction, s.config.Network)
	if err != nil {
		return "", nil, fmt.Errorf("hashing SEP-10 challenge transaction: %w", err)
	}
	encoded, err := transaction.Base64()
	if err != nil {
		return "", nil, fmt.Errorf("encoding SEP-10 challenge transaction: %w", err)
	}
	return encoded, append([]byte(nil), hash[:]...), nil
}

func (s *SEP10Signer) VerifyChallenge(challenge string) (string, []byte, error) {
	transaction, account, matchedHomeDomain, _, err := txnbuild.ReadChallengeTx(
		challenge, s.serverAccount, s.config.Network, s.config.WebAuthDomain, []string{s.config.HomeDomain},
	)
	if err != nil {
		return "", nil, fmt.Errorf("reading SEP-10 challenge transaction: %w", err)
	}
	if matchedHomeDomain != s.config.HomeDomain {
		return "", nil, errors.New("SEP-10 home domain mismatch")
	}
	if err := s.ValidateAccount(account); err != nil {
		return "", nil, err
	}
	if _, err := txnbuild.VerifyChallengeTxSigners(
		challenge, s.serverAccount, s.config.Network, s.config.WebAuthDomain,
		[]string{s.config.HomeDomain}, account,
	); err != nil {
		return "", nil, fmt.Errorf("verifying SEP-10 wallet signature: %w", err)
	}
	hash, err := sep10ChallengeHash(transaction, s.config.Network)
	if err != nil {
		return "", nil, fmt.Errorf("hashing signed SEP-10 challenge: %w", err)
	}
	return account, append([]byte(nil), hash[:]...), nil
}

func sep10ChallengeHash(transaction *txnbuild.Transaction, network string) ([32]byte, error) {
	unsigned, err := transaction.ClearSignatures()
	if err != nil {
		return [32]byte{}, fmt.Errorf("clearing SEP-10 challenge signatures: %w", err)
	}
	return unsigned.Hash(network)
}

var _ usecase.SEP10Signer = (*SEP10Signer)(nil)
