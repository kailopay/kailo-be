package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

var (
	ErrSEP10InvalidAccount    = errors.New("invalid sep-10 account")
	ErrSEP10InvalidChallenge  = errors.New("invalid sep-10 challenge")
	ErrSEP10ChallengeExpired  = errors.New("sep-10 challenge expired")
	ErrSEP10ChallengeConsumed = errors.New("sep-10 challenge already consumed")
	ErrSEP10AccountMismatch   = errors.New("sep-10 account mismatch")
	ErrSEP10InvalidToken      = errors.New("invalid sep-10 token")
	ErrSEP10ChallengeNotFound = errors.New("sep-10 challenge not found")
)

// SEP10Principal identifies the classic Stellar account authenticated by a
// SEP-10 bearer token.
type SEP10Principal struct {
	Account string
	TokenID string
}

type SEP10ChallengeView struct {
	Transaction string    `json:"transaction"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
}

type SEP10TokenView struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

type SEP10ChallengeRepository interface {
	Create(ctx context.Context, challenge entity.SEP10Challenge) error
	FindByHash(ctx context.Context, challengeHash []byte) (entity.SEP10Challenge, error)
	Consume(ctx context.Context, challengeID string, now time.Time) error
}

type SEP10Signer interface {
	BuildChallenge(account string) (challenge string, challengeHash []byte, err error)
	VerifyChallenge(challenge string) (account string, challengeHash []byte, err error)
	ValidateAccount(account string) error
}

type SEP10Dependencies struct {
	Challenges SEP10ChallengeRepository
	Signer     SEP10Signer
}

type SEP10ServiceConfig struct {
	Network       string
	HomeDomain    string
	ChallengeTTL  time.Duration
	TokenTTL      time.Duration
	TokenIssuer   string
	TokenAudience string
	TokenSecret   string
	Now           func() time.Time
	NewID         func() (string, error)
}

type SEP10Service interface {
	Challenge(ctx context.Context, account string) (SEP10ChallengeView, error)
	Exchange(ctx context.Context, signedChallenge string) (SEP10TokenView, error)
	Authenticate(ctx context.Context, token string) (SEP10Principal, error)
}

type SEP10Usecase struct {
	dependencies SEP10Dependencies
	config       SEP10ServiceConfig
	signer       jose.Signer
	key          []byte
}

func NewSEP10Usecase(dependencies SEP10Dependencies, config SEP10ServiceConfig) (*SEP10Usecase, error) {
	if dependencies.Challenges == nil || dependencies.Signer == nil || config.Now == nil || config.NewID == nil ||
		strings.TrimSpace(config.Network) == "" || strings.TrimSpace(config.HomeDomain) == "" ||
		config.ChallengeTTL <= 0 || config.TokenTTL <= 0 || strings.TrimSpace(config.TokenIssuer) == "" ||
		strings.TrimSpace(config.TokenAudience) == "" || strings.TrimSpace(config.TokenSecret) == "" {
		return nil, errors.New("valid SEP-10 dependencies and configuration are required")
	}
	key := sep10TokenKey(config.TokenSecret)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS256, Key: key}, nil)
	if err != nil {
		return nil, fmt.Errorf("creating SEP-10 token signer: %w", err)
	}
	return &SEP10Usecase{dependencies: dependencies, config: config, signer: signer, key: key}, nil
}

func (s *SEP10Usecase) Challenge(ctx context.Context, account string) (SEP10ChallengeView, error) {
	account = strings.TrimSpace(account)
	if account == "" || s.dependencies.Signer.ValidateAccount(account) != nil {
		return SEP10ChallengeView{}, ErrSEP10InvalidAccount
	}
	transaction, challengeHash, err := s.dependencies.Signer.BuildChallenge(account)
	if err != nil {
		return SEP10ChallengeView{}, fmt.Errorf("building SEP-10 challenge: %w", ErrSEP10InvalidChallenge)
	}
	if strings.TrimSpace(transaction) == "" || len(challengeHash) == 0 {
		return SEP10ChallengeView{}, ErrSEP10InvalidChallenge
	}
	now := s.config.Now().UTC()
	expiresAt := now.Add(s.config.ChallengeTTL)
	id, err := s.config.NewID()
	if err != nil {
		return SEP10ChallengeView{}, fmt.Errorf("generating SEP-10 challenge id: %w", err)
	}
	if err := s.dependencies.Challenges.Create(ctx, entity.SEP10Challenge{
		ID: id, ChallengeHash: append([]byte(nil), challengeHash...), Account: account,
		HomeDomain: s.config.HomeDomain, Network: s.config.Network, ExpiresAt: expiresAt, CreatedAt: now,
	}); err != nil {
		return SEP10ChallengeView{}, fmt.Errorf("storing SEP-10 challenge: %w", err)
	}
	return SEP10ChallengeView{Transaction: transaction, ExpiresAt: expiresAt}, nil
}

func (s *SEP10Usecase) Exchange(ctx context.Context, signedChallenge string) (SEP10TokenView, error) {
	signedChallenge = strings.TrimSpace(signedChallenge)
	if signedChallenge == "" || len(signedChallenge) > 1<<20 {
		return SEP10TokenView{}, ErrSEP10InvalidChallenge
	}
	account, challengeHash, err := s.dependencies.Signer.VerifyChallenge(signedChallenge)
	if err != nil {
		return SEP10TokenView{}, ErrSEP10InvalidChallenge
	}
	challenge, err := s.dependencies.Challenges.FindByHash(ctx, challengeHash)
	if err != nil {
		if errors.Is(err, ErrSEP10ChallengeNotFound) {
			return SEP10TokenView{}, ErrSEP10InvalidChallenge
		}
		return SEP10TokenView{}, fmt.Errorf("finding SEP-10 challenge: %w", err)
	}
	if !bytes.Equal(challenge.ChallengeHash, challengeHash) {
		return SEP10TokenView{}, ErrSEP10InvalidChallenge
	}
	now := s.config.Now().UTC()
	if challenge.ConsumedAt != nil {
		return SEP10TokenView{}, ErrSEP10ChallengeConsumed
	}
	if !now.Before(challenge.ExpiresAt) {
		return SEP10TokenView{}, ErrSEP10ChallengeExpired
	}
	if account != challenge.Account {
		return SEP10TokenView{}, ErrSEP10AccountMismatch
	}
	if err := s.dependencies.Challenges.Consume(ctx, challenge.ID, now); err != nil {
		if errors.Is(err, ErrSEP10ChallengeConsumed) {
			return SEP10TokenView{}, ErrSEP10ChallengeConsumed
		}
		return SEP10TokenView{}, fmt.Errorf("consuming SEP-10 challenge: %w", err)
	}
	tokenID, err := s.config.NewID()
	if err != nil {
		return SEP10TokenView{}, fmt.Errorf("generating SEP-10 token id: %w", err)
	}
	expiresAt := now.Add(s.config.TokenTTL)
	claims := sep10TokenClaims{
		Claims: jwt.Claims{
			Issuer:   s.config.TokenIssuer,
			Subject:  account,
			Audience: jwt.Audience{s.config.TokenAudience},
			Expiry:   jwt.NewNumericDate(expiresAt),
			IssuedAt: jwt.NewNumericDate(now),
			ID:       tokenID,
		},
		Account: account,
	}
	token, err := jwt.Signed(s.signer).Claims(claims).Serialize()
	if err != nil {
		return SEP10TokenView{}, fmt.Errorf("signing SEP-10 token: %w", err)
	}
	return SEP10TokenView{Token: token, ExpiresAt: expiresAt}, nil
}

func (s *SEP10Usecase) Authenticate(ctx context.Context, token string) (SEP10Principal, error) {
	_ = ctx
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 8192 {
		return SEP10Principal{}, ErrSEP10InvalidToken
	}
	signed, err := jose.ParseSigned(token, []jose.SignatureAlgorithm{jose.HS256})
	if err != nil {
		return SEP10Principal{}, ErrSEP10InvalidToken
	}
	payload, err := signed.Verify(s.key)
	if err != nil {
		return SEP10Principal{}, ErrSEP10InvalidToken
	}
	var claims sep10TokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return SEP10Principal{}, ErrSEP10InvalidToken
	}
	if err := claims.ValidateWithLeeway(jwt.Expected{
		Issuer:      s.config.TokenIssuer,
		AnyAudience: jwt.Audience{s.config.TokenAudience},
		Time:        s.config.Now().UTC(),
	}, 0); err != nil {
		return SEP10Principal{}, ErrSEP10InvalidToken
	}
	if claims.Account == "" || claims.Account != claims.Subject || claims.ID == "" ||
		s.dependencies.Signer.ValidateAccount(claims.Account) != nil {
		return SEP10Principal{}, ErrSEP10InvalidToken
	}
	return SEP10Principal{Account: claims.Account, TokenID: claims.ID}, nil
}

type sep10TokenClaims struct {
	jwt.Claims
	Account string `json:"account"`
}

func sep10TokenKey(secret string) []byte {
	sum := sha256.Sum256([]byte("kailopay/sep10/jwt/" + secret))
	return sum[:]
}
