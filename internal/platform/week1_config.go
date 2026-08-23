package platform

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

const StellarTestnetPassphrase = "Test SDF Network ; September 2015"

type Week1Config struct {
	APIKeyPepper  string
	Onramp        OnrampConfig
	CoinMarketCap CoinMarketCapConfig
	Xendit        XenditConfig
	Stellar       StellarConfig
	Worker        WorkerConfig
}

type OnrampConfig struct {
	QuoteTTL       time.Duration
	QuoteMaxAge    time.Duration
	QuoteSpreadBPS int
	MinIDR         int64
	MaxIDR         int64
}

type CoinMarketCapConfig struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

type XenditConfig struct {
	BaseURL       string
	SecretKey     string
	CallbackToken string
	APIVersion    string
	QRISChannel   string
	VAChannel     string
	Timeout       time.Duration
}

type StellarConfig struct {
	HorizonURL             string
	NetworkPassphrase      string
	TreasuryAccount        string
	OperatingBufferStroops int64
	Timeout                time.Duration
}

type WorkerConfig struct {
	PollInterval      time.Duration
	LeaseDuration     time.Duration
	RetryDelay        time.Duration
	SubmissionTimeout time.Duration
	MaxAttempts       int
}

func (cfg Week1Config) Validate() error {
	if len(cfg.APIKeyPepper) < 32 {
		return errors.New("API key pepper must be at least 32 bytes")
	}
	if cfg.Onramp.QuoteTTL <= 0 || cfg.Onramp.QuoteMaxAge <= 0 {
		return errors.New("quote durations must be positive")
	}
	if cfg.Onramp.QuoteSpreadBPS < 0 || cfg.Onramp.QuoteSpreadBPS > 10_000 {
		return errors.New("quote spread basis points are invalid")
	}
	if cfg.Onramp.MinIDR <= 0 || cfg.Onramp.MaxIDR < cfg.Onramp.MinIDR {
		return errors.New("onramp IDR range is invalid")
	}
	if err := validateHTTPSURL(cfg.CoinMarketCap.BaseURL); err != nil {
		return errors.New("CoinMarketCap base URL must use HTTPS")
	}
	if strings.TrimSpace(cfg.CoinMarketCap.APIKey) == "" || cfg.CoinMarketCap.Timeout <= 0 {
		return errors.New("CoinMarketCap credentials and timeout are required")
	}
	if err := validateHTTPSURL(cfg.Xendit.BaseURL); err != nil {
		return errors.New("Xendit base URL must use HTTPS")
	}
	if !strings.Contains(strings.ToLower(cfg.Xendit.SecretKey), "development") {
		return errors.New("Xendit development key is required")
	}
	if len(cfg.Xendit.CallbackToken) < 32 || cfg.Xendit.APIVersion != "2024-11-11" ||
		cfg.Xendit.QRISChannel != "QRIS" || cfg.Xendit.VAChannel != "BRI_VIRTUAL_ACCOUNT" ||
		cfg.Xendit.Timeout <= 0 {
		return errors.New("Xendit sandbox configuration is invalid")
	}
	if err := validateHTTPSURL(cfg.Stellar.HorizonURL); err != nil {
		return errors.New("Stellar Horizon URL must use HTTPS")
	}
	if cfg.Stellar.NetworkPassphrase != StellarTestnetPassphrase {
		return errors.New("Stellar testnet passphrase is required")
	}
	if strings.TrimSpace(cfg.Stellar.TreasuryAccount) == "" ||
		cfg.Stellar.OperatingBufferStroops < 0 || cfg.Stellar.Timeout <= 0 {
		return errors.New("Stellar treasury configuration is invalid")
	}
	if cfg.Worker.PollInterval <= 0 || cfg.Worker.LeaseDuration <= 0 ||
		cfg.Worker.RetryDelay <= 0 || cfg.Worker.SubmissionTimeout <= 0 || cfg.Worker.MaxAttempts <= 0 {
		return errors.New("worker configuration is invalid")
	}
	return nil
}

func validateHTTPSURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return errors.New("invalid HTTPS URL")
	}
	return nil
}
