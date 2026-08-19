package repository

import (
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
		"retail_sessions",
		"user_identities",
		"api_clients",
		"api_keys",
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

func TestIdentityModelsHaveExpectedTableNames(t *testing.T) {
	tests := []struct {
		model any
		want  string
	}{
		{model: User{}, want: "users"},
		{model: RetailSession{}, want: "retail_sessions"},
		{model: UserIdentity{}, want: "user_identities"},
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

func TestAuthSchemaUsesFederatedIdentityAndUserOwnedClients(t *testing.T) {
	userType := reflect.TypeOf(User{})
	for _, field := range []string{"EmailVerifiedAt", "DeveloperEnabledAt"} {
		if _, ok := userType.FieldByName(field); !ok {
			t.Errorf("User is missing %q", field)
		}
	}

	identityType := reflect.TypeOf(UserIdentity{})
	for _, field := range []string{"UserID", "Provider", "Subject"} {
		if _, ok := identityType.FieldByName(field); !ok {
			t.Errorf("UserIdentity is missing %q", field)
		}
	}

	clientType := reflect.TypeOf(APIClient{})
	if _, ok := clientType.FieldByName("OwnerUserID"); !ok {
		t.Error("APIClient is missing OwnerUserID")
	}
	if _, ok := clientType.FieldByName("OrganizationID"); ok {
		t.Error("APIClient must not retain OrganizationID")
	}

	orderType := reflect.TypeOf(Order{})
	if _, ok := orderType.FieldByName("OrganizationID"); ok {
		t.Error("Order must not retain OrganizationID")
	}

	webhookType := reflect.TypeOf(WebhookEndpoint{})
	if _, ok := webhookType.FieldByName("OrganizationID"); ok {
		t.Error("WebhookEndpoint must not retain OrganizationID")
	}
}
