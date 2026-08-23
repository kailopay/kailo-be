package stellar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/txnbuild"
)

const maxHorizonResponseBytes int64 = 2 << 20

type Config struct {
	HorizonURL         string
	NetworkPassphrase  string
	TreasurySecret     string
	HTTPClient         *http.Client
	TransactionTimeout time.Duration
}

type Client struct {
	baseURL    *url.URL
	passphrase string
	signer     *keypair.Full
	httpClient *http.Client
	txTimeout  time.Duration
}

func New(config Config) (*Client, error) {
	client, err := NewBalanceReader(config.HorizonURL, config.HTTPClient)
	if err != nil || config.NetworkPassphrase == "" || config.TransactionTimeout <= 0 {
		return nil, errors.New("valid Stellar configuration is required")
	}
	signer, err := keypair.ParseFull(config.TreasurySecret)
	if err != nil {
		return nil, errors.New("valid Stellar treasury secret is required")
	}
	client.passphrase = config.NetworkPassphrase
	client.signer = signer
	client.txTimeout = config.TransactionTimeout
	return client, nil
}

func NewBalanceReader(horizonURL string, httpClient *http.Client) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimRight(horizonURL, "/"))
	if err != nil || !baseURL.IsAbs() || httpClient == nil {
		return nil, errors.New("valid Stellar Horizon configuration is required")
	}
	return &Client{baseURL: baseURL, httpClient: httpClient}, nil
}

func (c *Client) Build(ctx context.Context, transfer usecase.Transfer) (usecase.BuiltTransaction, error) {
	if c.signer == nil || transfer.Source != c.signer.Address() || transfer.Amount.Validate() != nil || transfer.Destination == "" {
		return usecase.BuiltTransaction{}, errors.New("invalid Stellar transfer")
	}
	account, err := c.account(ctx, transfer.Source)
	if err != nil {
		return usecase.BuiltTransaction{}, err
	}
	source := txnbuild.NewSimpleAccount(transfer.Source, account.Sequence)
	memo := transfer.Memo
	if memo == "" {
		memo = "kp-" + strings.ReplaceAll(transfer.OrderID, "-", "")
		if len(memo) > txnbuild.MemoTextMaxLength {
			memo = memo[:txnbuild.MemoTextMaxLength]
		}
	}
	operation := &txnbuild.Payment{Destination: transfer.Destination, Amount: transfer.Amount.String(), Asset: txnbuild.NativeAsset{}}
	transaction, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount: &source, IncrementSequenceNum: true, Operations: []txnbuild.Operation{operation},
		BaseFee: txnbuild.MinBaseFee, Memo: txnbuild.MemoText(memo),
		Preconditions: txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(int64(c.txTimeout.Seconds()))},
	})
	if err != nil {
		return usecase.BuiltTransaction{}, fmt.Errorf("building Stellar transaction: %w", err)
	}
	signed, err := transaction.Sign(c.passphrase, c.signer)
	if err != nil {
		return usecase.BuiltTransaction{}, fmt.Errorf("signing Stellar transaction: %w", err)
	}
	hash, err := signed.HashHex(c.passphrase)
	if err != nil {
		return usecase.BuiltTransaction{}, fmt.Errorf("hashing Stellar transaction: %w", err)
	}
	envelope, err := signed.Base64()
	if err != nil {
		return usecase.BuiltTransaction{}, fmt.Errorf("encoding Stellar transaction: %w", err)
	}
	return usecase.BuiltTransaction{Hash: hash, Envelope: envelope}, nil
}

func (c *Client) ValidateDestination(account string) error {
	parsed, err := keypair.ParseAddress(account)
	if err != nil || parsed.Address() != account {
		return errors.New("invalid Stellar destination")
	}
	return nil
}

func (c *Client) Submit(ctx context.Context, transaction usecase.BuiltTransaction) (usecase.Submission, error) {
	form := url.Values{"tx": []string{transaction.Envelope}}
	request, err := c.request(ctx, http.MethodPost, "/transactions", strings.NewReader(form.Encode()))
	if err != nil {
		return usecase.Submission{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	status, body, err := c.do(request)
	if err != nil {
		return usecase.Submission{Result: usecase.SubmissionUnknown, SafeError: "Horizon submission unavailable"}, err
	}
	if status >= 500 {
		return usecase.Submission{Result: usecase.SubmissionUnknown, SafeError: "Horizon server error"}, nil
	}
	if status < 200 || status >= 300 {
		// A rejected sequence is a definitive pre-application failure: the
		// transaction never reached the ledger and rebuilding is safe.
		if status < 500 && transactionResultCode(body) == "tx_bad_seq" {
			return usecase.Submission{Result: usecase.SubmissionRetryable, SafeError: "Stellar sequence conflict"}, nil
		}
		return usecase.Submission{Result: usecase.SubmissionPermanent, SafeError: "Stellar transaction rejected"}, nil
	}
	var response struct {
		Hash       string `json:"hash"`
		CreatedAt  string `json:"created_at"`
		Successful bool   `json:"successful"`
	}
	if err := json.Unmarshal(body, &response); err != nil || response.Hash != transaction.Hash || !response.Successful {
		return usecase.Submission{Result: usecase.SubmissionUnknown, SafeError: "invalid Horizon submission response"}, nil
	}
	ledgerAt, _ := time.Parse(time.RFC3339Nano, response.CreatedAt)
	return usecase.Submission{Result: usecase.SubmissionConfirmed, LedgerAt: ledgerAt.UTC()}, nil
}

func (c *Client) FindByHash(ctx context.Context, hash string) (usecase.Submission, error) {
	request, err := c.request(ctx, http.MethodGet, "/transactions/"+url.PathEscape(hash), nil)
	if err != nil {
		return usecase.Submission{}, err
	}
	status, body, err := c.do(request)
	if err != nil {
		return usecase.Submission{}, err
	}
	if status == http.StatusNotFound {
		return usecase.Submission{Result: usecase.SubmissionPending}, nil
	}
	if status < 200 || status >= 300 {
		return usecase.Submission{Result: usecase.SubmissionUnknown}, nil
	}
	var response struct {
		Hash       string `json:"hash"`
		CreatedAt  string `json:"created_at"`
		Successful bool   `json:"successful"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return usecase.Submission{}, err
	}
	if response.Hash != hash {
		return usecase.Submission{Result: usecase.SubmissionUnknown, SafeError: "Horizon reconciliation hash mismatch"}, nil
	}
	if !response.Successful {
		return usecase.Submission{Result: usecase.SubmissionPermanent, SafeError: "Stellar transaction failed"}, nil
	}
	ledgerAt, _ := time.Parse(time.RFC3339Nano, response.CreatedAt)
	return usecase.Submission{Result: usecase.SubmissionConfirmed, LedgerAt: ledgerAt.UTC()}, nil
}

func transactionResultCode(body []byte) string {
	var payload struct {
		Error struct {
			Extras struct {
				ResultCodes struct {
					Transaction string `json:"transaction"`
				} `json:"result_codes"`
			} `json:"extras"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return payload.Error.Extras.ResultCodes.Transaction
}

func (c *Client) SpendableBalance(ctx context.Context, accountID string) (entity.Stroops, error) {
	account, err := c.account(ctx, accountID)
	if err != nil {
		return 0, err
	}
	request, err := c.request(ctx, http.MethodGet, "/ledgers?order=desc&limit=1", nil)
	if err != nil {
		return 0, err
	}
	status, body, err := c.do(request)
	if err != nil || status < 200 || status >= 300 {
		return 0, errors.New("reading Stellar base reserve")
	}
	var ledgers struct {
		Embedded struct {
			Records []struct {
				BaseReserve int64 `json:"base_reserve_in_stroops"`
			} `json:"records"`
		} `json:"_embedded"`
	}
	if err := json.Unmarshal(body, &ledgers); err != nil || len(ledgers.Embedded.Records) == 0 {
		return 0, errors.New("invalid Stellar ledger response")
	}
	minimumReserve := int64(2+account.SubentryCount+account.NumSponsoring-account.NumSponsored) * ledgers.Embedded.Records[0].BaseReserve
	spendable := account.NativeBalance - account.NativeSellingLiabilities - minimumReserve
	if spendable <= 0 {
		return 0, errors.New("Stellar account has no spendable balance")
	}
	return entity.Stroops(spendable), nil
}

type horizonAccount struct {
	Sequence                 int64 `json:"sequence,string"`
	SubentryCount            int64 `json:"subentry_count"`
	NumSponsoring            int64 `json:"num_sponsoring"`
	NumSponsored             int64 `json:"num_sponsored"`
	NativeBalance            int64
	NativeSellingLiabilities int64
}

func (c *Client) account(ctx context.Context, accountID string) (horizonAccount, error) {
	request, err := c.request(ctx, http.MethodGet, "/accounts/"+url.PathEscape(accountID), nil)
	if err != nil {
		return horizonAccount{}, err
	}
	status, body, err := c.do(request)
	if err != nil || status < 200 || status >= 300 {
		return horizonAccount{}, errors.New("reading Stellar account")
	}
	var payload struct {
		Sequence      string `json:"sequence"`
		SubentryCount int64  `json:"subentry_count"`
		NumSponsoring int64  `json:"num_sponsoring"`
		NumSponsored  int64  `json:"num_sponsored"`
		Balances      []struct {
			AssetType          string `json:"asset_type"`
			Balance            string `json:"balance"`
			SellingLiabilities string `json:"selling_liabilities"`
		} `json:"balances"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return horizonAccount{}, errors.New("invalid Stellar account response")
	}
	sequence, err := strconv.ParseInt(payload.Sequence, 10, 64)
	if err != nil {
		return horizonAccount{}, errors.New("invalid Stellar account sequence")
	}
	result := horizonAccount{Sequence: sequence, SubentryCount: payload.SubentryCount, NumSponsoring: payload.NumSponsoring, NumSponsored: payload.NumSponsored}
	for _, balance := range payload.Balances {
		if balance.AssetType == "native" {
			result.NativeBalance, err = stroops(balance.Balance)
			if err != nil {
				return horizonAccount{}, err
			}
			if balance.SellingLiabilities != "" {
				result.NativeSellingLiabilities, err = stroops(balance.SellingLiabilities)
			}
			return result, err
		}
	}
	return horizonAccount{}, errors.New("native XLM balance missing")
}

func (c *Client) request(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	endpoint := *c.baseURL
	if strings.Contains(path, "?") {
		parts := strings.SplitN(path, "?", 2)
		endpoint.Path = strings.TrimRight(endpoint.Path, "/") + parts[0]
		endpoint.RawQuery = parts[1]
	} else {
		endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	}
	return http.NewRequestWithContext(ctx, method, endpoint.String(), body)
}

func (c *Client) do(request *http.Request) (int, []byte, error) {
	response, err := c.httpClient.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxHorizonResponseBytes+1))
	if err != nil || int64(len(body)) > maxHorizonResponseBytes {
		return response.StatusCode, nil, errors.New("invalid Horizon response")
	}
	return response.StatusCode, body, nil
}

func stroops(decimal string) (int64, error) {
	value, ok := new(big.Rat).SetString(decimal)
	if !ok || value.Sign() < 0 {
		return 0, errors.New("invalid XLM amount")
	}
	value.Mul(value, new(big.Rat).SetInt64(int64(entity.StroopsPerXLM)))
	if !value.IsInt() || !value.Num().IsInt64() {
		return 0, errors.New("XLM amount exceeds stroop precision")
	}
	return value.Num().Int64(), nil
}
