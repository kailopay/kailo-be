package repository

import "github.com/febry3/kailopay-be/internal/entity"

func MigrationModels() []any {
	return []any{
		&entity.User{},
		&entity.UserIdentity{},
		&entity.AuthTransaction{},
		&entity.AuthCredential{},
		&entity.AuthChallenge{},
		&entity.RetailSession{},
		&entity.APIClient{},
		&entity.APIKey{},
		&entity.OrderRecord{},
		&entity.OrderEvent{},
		&entity.PaymentCheckout{},
		&entity.GatewayEvent{},
		&entity.StellarTransaction{},
		&entity.WebhookEndpoint{},
		&entity.WebhookEvent{},
		&entity.WebhookAttempt{},
		&entity.IdempotencyRecord{},
		&entity.OutboxMessage{},
		&entity.TreasuryAccount{},
		&entity.TreasuryReservation{},
		&entity.OfframpPayout{},
	}
}
