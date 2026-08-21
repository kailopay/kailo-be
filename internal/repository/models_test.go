package repository

import (
	"github.com/febry3/kailopay-be/internal/entity"
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
		"retail_sessions",
		"user_identities",
		"api_clients",
		"api_keys",
		"treasury_accounts",
		"treasury_reservations",
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
