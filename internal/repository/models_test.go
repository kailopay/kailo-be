package repository

import (
	"sync"

	"github.com/febry3/kailopay-be/internal/entity"
	"gorm.io/gorm/schema"
	"reflect"
	"testing"
)

func TestMigrationModelsAreExplicitlyRegistered(t *testing.T) {
	models := MigrationModels()
	if len(models) == 0 {
		t.Fatal("MigrationModels() returned no models")
	}

	registeredTables := make(map[string]struct{}, len(models))
	for _, model := range models {
		tableNamer, ok := model.(interface{ TableName() string })
		if !ok {
			t.Fatalf("model %T does not expose a table name", model)
		}
		registeredTables[tableNamer.TableName()] = struct{}{}
	}

	for _, table := range []string{
		"users",
		"auth_transactions",
		"auth_credentials",
		"auth_challenges",
		"retail_sessions",
		"user_identities",
		"api_clients",
		"api_keys",
		"treasury_accounts",
		"treasury_reservations",
		"sep24_transactions",
		"sep10_challenges",
		"sep24_interactive_sessions",
		"sep38_quotes",
	} {
		if _, ok := registeredTables[table]; !ok {
			t.Errorf("MigrationModels() is missing %q", table)
		}
	}

	for _, table := range []string{"organizations", "organization_memberships"} {
		if _, ok := registeredTables[table]; ok {
			t.Errorf("MigrationModels() must not register obsolete table %q", table)
		}
	}
}

func TestSEPWalletModelsExposeStableCorrelationFields(t *testing.T) {
	tests := []struct {
		name   string
		model  any
		table  string
		fields []string
	}{
		{name: "sep10 challenge", model: entity.SEP10Challenge{}, table: "sep10_challenges", fields: []string{"ID", "ChallengeHash", "Account", "HomeDomain", "Network", "ExpiresAt", "ConsumedAt"}},
		{name: "sep24 interactive session", model: entity.SEP24InteractiveSession{}, table: "sep24_interactive_sessions", fields: []string{"ID", "TransactionID", "Kind", "WalletAccount", "BrowserTokenHash", "RequestHash", "RequestPayload", "KYCStatus", "OrderID"}},
		{name: "sep38 quote", model: entity.SEP38Quote{}, table: "sep38_quotes", fields: []string{"ID", "QuoteID", "WalletAccount", "SellAsset", "BuyAsset", "SellAmount", "BuyAmount", "ExpiresAt", "ConsumedAt"}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			typeOf := reflect.TypeOf(testCase.model)
			for _, field := range testCase.fields {
				if _, ok := typeOf.FieldByName(field); !ok {
					t.Errorf("model %T is missing %q", testCase.model, field)
				}
			}
			tableNamer, ok := testCase.model.(interface{ TableName() string })
			if !ok {
				t.Fatalf("model %T does not expose TableName", testCase.model)
			}
			if got := tableNamer.TableName(); got != testCase.table {
				t.Fatalf("TableName() = %q, want %q", got, testCase.table)
			}
		})
	}
}

func TestProtocolCorrelationFieldsAreNullableAndUnique(t *testing.T) {
	sep24Type := reflect.TypeOf(entity.SEP24Transaction{})
	for _, field := range []string{"WalletAccount", "QuoteID", "StellarTransactionID", "ExternalTransactionID"} {
		if _, ok := sep24Type.FieldByName(field); !ok {
			t.Errorf("entity.SEP24Transaction is missing %q", field)
		}
	}
	orderType := reflect.TypeOf(entity.OrderRecord{})
	for _, field := range []string{"WalletAccount", "QuoteID"} {
		if _, ok := orderType.FieldByName(field); !ok {
			t.Errorf("entity.OrderRecord is missing %q", field)
		}
	}

	for _, testCase := range []struct {
		name  string
		model any
		field string
	}{
		{name: "sep10 challenge hash", model: &entity.SEP10Challenge{}, field: "ChallengeHash"},
		{name: "sep24 interactive transaction id", model: &entity.SEP24InteractiveSession{}, field: "TransactionID"},
		{name: "sep38 quote id", model: &entity.SEP38Quote{}, field: "QuoteID"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			modelSchema, err := schema.Parse(testCase.model, &sync.Map{}, schema.NamingStrategy{})
			if err != nil {
				t.Fatalf("schema.Parse() error = %v", err)
			}
			field := modelSchema.LookUpField(testCase.field)
			if field == nil {
				t.Fatalf("field %q not found", testCase.field)
			}
			if !field.Unique {
				t.Fatalf("field %q must use a unique constraint", testCase.field)
			}
		})
	}
}

func TestSEP24TransactionModelHasStableCorrelationFields(t *testing.T) {
	typeOf := reflect.TypeOf(entity.SEP24Transaction{})
	for _, field := range []string{"ID", "TransactionID", "OrderID", "Kind", "CreatedAt"} {
		if _, ok := typeOf.FieldByName(field); !ok {
			t.Errorf("entity.SEP24Transaction is missing %q", field)
		}
	}
	if got := (entity.SEP24Transaction{}).TableName(); got != "sep24_transactions" {
		t.Fatalf("TableName() = %q, want sep24_transactions", got)
	}
}

func TestOnrampModelsUseExactQuoteAndTreasuryFields(t *testing.T) {
	orderType := reflect.TypeOf(entity.OrderRecord{})
	for _, field := range []string{
		"QuoteProvider",
		"QuoteSourceAt",
		"QuoteRate",
		"QuoteAdjustedRate",
		"QuoteSpreadBPS",
		"AssetAmountStroops",
		"QuoteExpiresAt",
	} {
		if _, ok := orderType.FieldByName(field); !ok {
			t.Errorf("entity.OrderRecord is missing %q", field)
		}
	}

	reservationType := reflect.TypeOf(entity.TreasuryReservation{})
	for _, field := range []string{"OrderID", "AmountStroops", "Status", "ExpiresAt"} {
		if _, ok := reservationType.FieldByName(field); !ok {
			t.Errorf("entity.TreasuryReservation is missing %q", field)
		}
	}
}

func TestIdentityModelsHaveExpectedTableNames(t *testing.T) {
	tests := []struct {
		model any
		want  string
	}{
		{model: entity.User{}, want: "users"},
		{model: entity.AuthTransaction{}, want: "auth_transactions"},
		{model: entity.RetailSession{}, want: "retail_sessions"},
		{model: entity.UserIdentity{}, want: "user_identities"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := reflect.ValueOf(tt.model).MethodByName("TableName").Call(nil)[0].String()
			if got != tt.want {
				t.Fatalf("TableName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAuthTransactionHasOneTimeCallbackFields(t *testing.T) {
	typeOf := reflect.TypeOf(entity.AuthTransaction{})
	for _, field := range []string{"StateHash", "NonceHash", "CodeVerifierCiphertext", "ExpiresAt", "ConsumedAt"} {
		if _, ok := typeOf.FieldByName(field); !ok {
			t.Errorf("entity.AuthTransaction is missing %q", field)
		}
	}
}

func TestMigrationBackedScalarUniqueFieldsUseUniqueConstraints(t *testing.T) {
	tests := []struct {
		name  string
		model any
		field string
	}{
		{name: "auth transaction state hash", model: &entity.AuthTransaction{}, field: "StateHash"},
		{name: "retail session token hash", model: &entity.RetailSession{}, field: "TokenHash"},
		{name: "api key public id", model: &entity.APIKey{}, field: "PublicID"},
		{name: "treasury reservation order id", model: &entity.TreasuryReservation{}, field: "OrderID"},
		{name: "stellar transaction intent id", model: &entity.StellarTransaction{}, field: "IntentID"},
		{name: "stellar transaction hash", model: &entity.StellarTransaction{}, field: "TransactionHash"},
		{name: "kyc provider request key", model: &entity.KYCInquiry{}, field: "ProviderRequestKey"},
		{name: "offramp payout order id", model: &entity.OfframpPayout{}, field: "OrderID"},
		{name: "offramp payout reference id", model: &entity.OfframpPayout{}, field: "ReferenceID"},
		{name: "sep24 transaction id", model: &entity.SEP24Transaction{}, field: "TransactionID"},
		{name: "sep24 order id", model: &entity.SEP24Transaction{}, field: "OrderID"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			modelSchema, err := schema.Parse(test.model, &sync.Map{}, schema.NamingStrategy{})
			if err != nil {
				t.Fatalf("schema.Parse() error = %v", err)
			}
			field := modelSchema.LookUpField(test.field)
			if field == nil {
				t.Fatalf("field %q not found", test.field)
			}
			if !field.Unique {
				t.Fatalf("field %q must use a unique constraint for migration compatibility", test.field)
			}
		})
	}
}

func TestAuthSchemaUsesFederatedIdentityAndUserOwnedClients(t *testing.T) {
	userType := reflect.TypeOf(entity.User{})
	for _, field := range []string{"EmailVerifiedAt", "DeveloperEnabledAt"} {
		if _, ok := userType.FieldByName(field); !ok {
			t.Errorf("entity.User is missing %q", field)
		}
	}

	identityType := reflect.TypeOf(entity.UserIdentity{})
	for _, field := range []string{"UserID", "Provider", "Subject"} {
		if _, ok := identityType.FieldByName(field); !ok {
			t.Errorf("entity.UserIdentity is missing %q", field)
		}
	}

	clientType := reflect.TypeOf(entity.APIClient{})
	if _, ok := clientType.FieldByName("OwnerUserID"); !ok {
		t.Error("entity.APIClient is missing OwnerUserID")
	}
	if _, ok := clientType.FieldByName("OrganizationID"); ok {
		t.Error("entity.APIClient must not retain OrganizationID")
	}

	orderType := reflect.TypeOf(entity.OrderRecord{})
	if _, ok := orderType.FieldByName("OrganizationID"); ok {
		t.Error("entity.OrderRecord must not retain OrganizationID")
	}

	webhookType := reflect.TypeOf(entity.WebhookEndpoint{})
	if _, ok := webhookType.FieldByName("OrganizationID"); ok {
		t.Error("entity.WebhookEndpoint must not retain OrganizationID")
	}
}

func TestKYCMigrationModelsAreRegistered(t *testing.T) {
	registeredTables := make(map[string]struct{})
	for _, model := range MigrationModels() {
		tableNamer, ok := model.(interface{ TableName() string })
		if !ok {
			continue
		}
		registeredTables[tableNamer.TableName()] = struct{}{}
	}
	for _, table := range []string{"kyc_inquiries", "kyc_provider_events"} {
		if _, ok := registeredTables[table]; !ok {
			t.Errorf("MigrationModels() is missing %q", table)
		}
	}
}

func TestWeek3DeveloperExperienceModelsAreRegistered(t *testing.T) {
	registeredTables := make(map[string]struct{})
	for _, model := range MigrationModels() {
		tableNamer, ok := model.(interface{ TableName() string })
		if !ok {
			continue
		}
		registeredTables[tableNamer.TableName()] = struct{}{}
	}
	for _, table := range []string{"order_financials", "developer_wallets"} {
		if _, ok := registeredTables[table]; !ok {
			t.Errorf("MigrationModels() is missing %q", table)
		}
	}
}

func TestWeek3FinancialAndWalletModelsExposeOwnershipAndAuditFields(t *testing.T) {
	financialType := reflect.TypeOf(entity.OrderFinancial{})
	for _, field := range []string{
		"ID", "OrderID", "ClientID", "Direction", "Environment", "Currency",
		"GrossAmountMinor", "FeeAmountMinor", "PlatformRevenueMinor",
		"DeveloperRevenueMinor", "NetAmountMinor", "FeeCurrency", "FeePolicyVersion",
		"Source", "Simulated", "CreatedAt",
	} {
		if _, ok := financialType.FieldByName(field); !ok {
			t.Errorf("entity.OrderFinancial is missing %q", field)
		}
	}
	if got := (entity.OrderFinancial{}).TableName(); got != "order_financials" {
		t.Fatalf("OrderFinancial.TableName() = %q, want order_financials", got)
	}

	walletType := reflect.TypeOf(entity.DeveloperWallet{})
	for _, field := range []string{
		"ID", "UserID", "ClientID", "Network", "WalletAccount", "Label",
		"IsPrimary", "VerificationMethod", "VerifiedAt", "Status", "CreatedAt", "UpdatedAt",
	} {
		if _, ok := walletType.FieldByName(field); !ok {
			t.Errorf("entity.DeveloperWallet is missing %q", field)
		}
	}
	if got := (entity.DeveloperWallet{}).TableName(); got != "developer_wallets" {
		t.Fatalf("DeveloperWallet.TableName() = %q, want developer_wallets", got)
	}
}

func TestWeek3ModelUniqueConstraintsProtectFinancialAndWalletOwnership(t *testing.T) {
	financialSchema, err := schema.Parse(&entity.OrderFinancial{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("financial schema.Parse() error = %v", err)
	}
	if field := financialSchema.LookUpField("OrderID"); field == nil || !field.Unique {
		t.Fatal("OrderFinancial.OrderID must be unique")
	}

	walletSchema, err := schema.Parse(&entity.DeveloperWallet{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("wallet schema.Parse() error = %v", err)
	}
	index := walletSchema.LookIndex("idx_developer_wallet_owner_network_account")
	if index == nil || index.Class != "UNIQUE" {
		t.Fatal("DeveloperWallet owner/network/account index must be unique")
	}
	if len(index.Fields) != 3 {
		t.Fatalf("DeveloperWallet owner/network/account index fields = %d, want 3", len(index.Fields))
	}
	for fieldIndex, fieldName := range []string{"UserID", "Network", "WalletAccount"} {
		if got := index.Fields[fieldIndex].Name; got != fieldName {
			t.Errorf("DeveloperWallet unique index field %d = %q, want %q", fieldIndex, got, fieldName)
		}
	}
}

func TestKYCModelsHaveAuditableFieldsWithoutRawPayload(t *testing.T) {
	inquiryType := reflect.TypeOf(entity.KYCInquiry{})
	for _, field := range []string{
		"ID", "UserID", "Provider", "ProviderInquiryID", "ProviderRequestKey", "ProviderStatus",
		"Status", "ProviderEventAt", "LastProviderEventID", "ApprovedAt", "ExpiresAt", "CreatedAt", "UpdatedAt",
	} {
		if _, ok := inquiryType.FieldByName(field); !ok {
			t.Errorf("entity.KYCInquiry is missing %q", field)
		}
	}

	eventType := reflect.TypeOf(entity.KYCProviderEvent{})
	for _, field := range []string{
		"ID", "Provider", "ProviderEventID", "InquiryID", "EventType", "ProviderEventAt", "PayloadHash", "ReceivedAt",
	} {
		if _, ok := eventType.FieldByName(field); !ok {
			t.Errorf("entity.KYCProviderEvent is missing %q", field)
		}
	}
	if _, ok := eventType.FieldByName("RawPayload"); ok {
		t.Error("entity.KYCProviderEvent must not persist RawPayload")
	}
}

func TestKYCModelsUseExpectedTableNames(t *testing.T) {
	for _, testCase := range []struct {
		model any
		want  string
	}{
		{model: entity.KYCInquiry{}, want: "kyc_inquiries"},
		{model: entity.KYCProviderEvent{}, want: "kyc_provider_events"},
	} {
		t.Run(testCase.want, func(t *testing.T) {
			got := reflect.ValueOf(testCase.model).MethodByName("TableName").Call(nil)[0].String()
			if got != testCase.want {
				t.Fatalf("TableName() = %q, want %q", got, testCase.want)
			}
		})
	}
}
