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
