package platform

import (
	"strings"
	"testing"
	"time"
)

func validWeek1Config() Week1Config {
	return Week1Config{
		APIKeyPepper: strings.Repeat("p", 32),
		Onramp: OnrampConfig{
			QuoteTTL:       5 * time.Minute,
			QuoteMaxAge:    2 * time.Minute,
			QuoteSpreadBPS: 0,
			MinIDR:         10_000,
			MaxIDR:         10_000_000,
		},
		CoinMarketCap: CoinMarketCapConfig{
			BaseURL: "https://pro-api.coinmarketcap.com",
			APIKey:  "test-market-data-key",
			Timeout: 5 * time.Second,
		},
		Xendit: XenditConfig{
			BaseURL:       "https://api.xendit.co",
			SecretKey:     "xnd_development_test",
			CallbackToken: strings.Repeat("x", 32),
			APIVersion:    "2024-11-11",
			QRISChannel:   "QRIS",
			VAChannel:     "BRI_VIRTUAL_ACCOUNT",
			Timeout:       10 * time.Second,
		},
		Stellar: StellarConfig{
			HorizonURL:             "https://horizon-testnet.stellar.org",
			NetworkPassphrase:      "Test SDF Network ; September 2015",
			TreasuryAccount:        "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
			OperatingBufferStroops: 10_000_000,
			Timeout:                10 * time.Second,
		},
		Worker: WorkerConfig{
			PollInterval:  time.Second,
			LeaseDuration: 30 * time.Second,
			RetryDelay:    5 * time.Second,
			MaxAttempts:   5,
		},
	}
}

func TestWeek1ConfigRejectsUnsafeEnvironment(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Week1Config)
	}{
		{name: "production Xendit key", mutate: func(cfg *Week1Config) { cfg.Xendit.SecretKey = "xnd_production_secret" }},
		{name: "mainnet passphrase", mutate: func(cfg *Week1Config) {
			cfg.Stellar.NetworkPassphrase = "Public Global Stellar Network ; September 2015"
		}},
		{name: "invalid spread", mutate: func(cfg *Week1Config) { cfg.Onramp.QuoteSpreadBPS = 10_001 }},
		{name: "invalid amount range", mutate: func(cfg *Week1Config) { cfg.Onramp.MaxIDR = cfg.Onramp.MinIDR - 1 }},
		{name: "short API key pepper", mutate: func(cfg *Week1Config) { cfg.APIKeyPepper = "short" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validWeek1Config()
			tt.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}
