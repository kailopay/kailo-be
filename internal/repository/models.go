package repository

import "time"

type User struct {
	ID                 string  `gorm:"type:uuid;primaryKey"`
	Status             string  `gorm:"type:text;not null;index"`
	DisplayName        string  `gorm:"type:text;not null"`
	Email              *string `gorm:"type:text"`
	EmailVerifiedAt    *time.Time
	DeveloperEnabledAt *time.Time
	CreatedAt          time.Time `gorm:"not null"`
	UpdatedAt          time.Time `gorm:"not null"`
}

func (User) TableName() string { return "users" }

type UserIdentity struct {
	ID          string    `gorm:"type:uuid;primaryKey"`
	UserID      string    `gorm:"type:uuid;not null;index"`
	Provider    string    `gorm:"type:text;not null;uniqueIndex:idx_user_identity_provider_subject,priority:1"`
	Subject     string    `gorm:"type:text;not null;uniqueIndex:idx_user_identity_provider_subject,priority:2"`
	CreatedAt   time.Time `gorm:"not null"`
	LastLoginAt *time.Time
}

func (UserIdentity) TableName() string { return "user_identities" }

type RetailSession struct {
	ID         string     `gorm:"type:uuid;primaryKey"`
	UserID     string     `gorm:"type:uuid;not null;index"`
	TokenHash  []byte     `gorm:"type:bytea;not null;uniqueIndex"`
	ExpiresAt  time.Time  `gorm:"not null;index"`
	LastUsedAt *time.Time `gorm:"index"`
	RevokedAt  *time.Time `gorm:"index"`
	CreatedAt  time.Time  `gorm:"not null"`
	UpdatedAt  time.Time  `gorm:"not null"`
}

func (RetailSession) TableName() string { return "retail_sessions" }

type APIClient struct {
	ID          string    `gorm:"type:uuid;primaryKey"`
	OwnerUserID string    `gorm:"type:uuid;not null;index"`
	Name        string    `gorm:"type:text;not null"`
	Environment string    `gorm:"type:text;not null"`
	Status      string    `gorm:"type:text;not null"`
	CreatedAt   time.Time `gorm:"not null"`
	UpdatedAt   time.Time `gorm:"not null"`
}

func (APIClient) TableName() string { return "api_clients" }

type APIKey struct {
	ID         string    `gorm:"type:uuid;primaryKey"`
	ClientID   string    `gorm:"type:uuid;not null;index"`
	PublicID   string    `gorm:"type:text;not null;uniqueIndex"`
	Prefix     string    `gorm:"type:text;not null"`
	SecretHash []byte    `gorm:"type:bytea;not null"`
	CreatedAt  time.Time `gorm:"not null"`
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

func (APIKey) TableName() string { return "api_keys" }

type Order struct {
	ID                    string  `gorm:"type:uuid;primaryKey"`
	ClientID              *string `gorm:"type:uuid;index"`
	CreatedByUserID       *string `gorm:"type:uuid;index"`
	RetailSessionID       *string `gorm:"type:uuid;index"`
	Direction             string  `gorm:"type:text;not null"`
	Status                string  `gorm:"type:text;not null;index"`
	Version               int     `gorm:"not null"`
	Currency              string  `gorm:"type:text;not null"`
	FiatAmountMinor       int64   `gorm:"not null"`
	AssetCode             string  `gorm:"type:text;not null"`
	AssetIssuer           string  `gorm:"type:text;not null"`
	Network               string  `gorm:"type:text;not null"`
	AssetAmount           string  `gorm:"type:numeric(30,18);not null"`
	PaymentMethod         string  `gorm:"type:text"`
	GatewayProvider       string  `gorm:"type:text"`
	StellarSource         *string `gorm:"type:text"`
	StellarDestination    *string `gorm:"type:text"`
	StellarMemo           *string `gorm:"type:text"`
	WithdrawalDestination []byte  `gorm:"type:jsonb"`
	ExpiresAt             *time.Time
	FailureCode           *string `gorm:"type:text"`
	FailureStage          *string `gorm:"type:text"`
	FailureRetryable      *bool
	CreatedAt             time.Time `gorm:"not null;index"`
	UpdatedAt             time.Time `gorm:"not null;index"`
	CompletedAt           *time.Time
}

func (Order) TableName() string { return "orders" }

type OrderEvent struct {
	ID               string    `gorm:"type:uuid;primaryKey"`
	OrderID          string    `gorm:"type:uuid;not null;uniqueIndex:idx_order_event_version,priority:1"`
	AggregateVersion int       `gorm:"not null;uniqueIndex:idx_order_event_version,priority:2"`
	EventType        string    `gorm:"type:text;not null"`
	PreviousStatus   *string   `gorm:"type:text"`
	NewStatus        *string   `gorm:"type:text"`
	Source           string    `gorm:"type:text;not null"`
	CorrelationID    string    `gorm:"type:text;not null;index"`
	Metadata         []byte    `gorm:"type:jsonb"`
	CreatedAt        time.Time `gorm:"not null"`
}

func (OrderEvent) TableName() string { return "order_events" }

type PaymentCheckout struct {
	ID                    string  `gorm:"type:uuid;primaryKey"`
	OrderID               string  `gorm:"type:uuid;not null;index:idx_payment_order_created,priority:1"`
	Provider              string  `gorm:"type:text;not null;uniqueIndex:idx_payment_provider_checkout,priority:1"`
	ProviderCheckoutID    string  `gorm:"type:text;not null;uniqueIndex:idx_payment_provider_checkout,priority:2"`
	Method                string  `gorm:"type:text;not null"`
	Currency              string  `gorm:"type:text;not null"`
	AmountMinor           int64   `gorm:"not null"`
	Status                string  `gorm:"type:text;not null"`
	PresentationReference *string `gorm:"type:text"`
	ExpiresAt             *time.Time
	Metadata              []byte    `gorm:"type:jsonb"`
	CreatedAt             time.Time `gorm:"not null;index:idx_payment_order_created,priority:2"`
	UpdatedAt             time.Time `gorm:"not null"`
}

func (PaymentCheckout) TableName() string { return "payment_checkouts" }

type GatewayEvent struct {
	ID                string    `gorm:"type:uuid;primaryKey"`
	Provider          string    `gorm:"type:text;not null;uniqueIndex:idx_gateway_event,priority:1"`
	ProviderEventID   string    `gorm:"type:text;not null;uniqueIndex:idx_gateway_event,priority:2"`
	EventType         string    `gorm:"type:text;not null"`
	CheckoutReference *string   `gorm:"type:text"`
	OrderReference    *string   `gorm:"type:text"`
	PayloadHash       string    `gorm:"type:text;not null"`
	SignatureVerified bool      `gorm:"not null"`
	MatchingResult    string    `gorm:"type:text"`
	ReceivedAt        time.Time `gorm:"not null"`
	ProcessedAt       *time.Time
	ProcessingStatus  string  `gorm:"type:text;not null"`
	ProcessingError   *string `gorm:"type:text"`
}

func (GatewayEvent) TableName() string { return "gateway_events" }

type StellarTransaction struct {
	ID              string  `gorm:"type:uuid;primaryKey"`
	OrderID         string  `gorm:"type:uuid;not null;index"`
	IntentID        string  `gorm:"type:text;not null;uniqueIndex"`
	Purpose         string  `gorm:"type:text;not null"`
	Network         string  `gorm:"type:text;not null"`
	AssetCode       string  `gorm:"type:text;not null"`
	Amount          string  `gorm:"type:numeric(30,18);not null"`
	Source          *string `gorm:"type:text"`
	Destination     *string `gorm:"type:text"`
	Memo            *string `gorm:"type:text"`
	TransactionHash *string `gorm:"type:text;uniqueIndex"`
	Status          string  `gorm:"type:text;not null"`
	AttemptCount    int     `gorm:"not null"`
	LedgerAt        *time.Time
	LastError       *string   `gorm:"type:text"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
}

func (StellarTransaction) TableName() string { return "stellar_transactions" }

type WebhookEndpoint struct {
	ID              string    `gorm:"type:uuid;primaryKey"`
	ClientID        string    `gorm:"type:uuid;not null;index"`
	URL             string    `gorm:"type:text;not null"`
	Status          string    `gorm:"type:text;not null"`
	SecretReference string    `gorm:"type:text;not null"`
	EventTypes      []byte    `gorm:"type:jsonb;not null"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
	DisabledAt      *time.Time
}

func (WebhookEndpoint) TableName() string { return "webhook_endpoints" }

type WebhookEvent struct {
	ID                 string    `gorm:"type:uuid;primaryKey"`
	OrderID            string    `gorm:"type:uuid;not null;index"`
	EventType          string    `gorm:"type:text;not null"`
	APIVersion         string    `gorm:"type:text;not null"`
	CanonicalPayload   []byte    `gorm:"type:jsonb;not null"`
	SourceOrderEventID *string   `gorm:"type:uuid"`
	CreatedAt          time.Time `gorm:"not null"`
}

func (WebhookEvent) TableName() string { return "webhook_events" }

type WebhookAttempt struct {
	ID               string    `gorm:"type:uuid;primaryKey"`
	EventID          string    `gorm:"type:uuid;not null;uniqueIndex:idx_webhook_attempt,priority:1"`
	EndpointID       string    `gorm:"type:uuid;not null;uniqueIndex:idx_webhook_attempt,priority:2"`
	AttemptNumber    int       `gorm:"not null;uniqueIndex:idx_webhook_attempt,priority:3"`
	Status           string    `gorm:"type:text;not null"`
	ScheduledAt      time.Time `gorm:"not null;index"`
	StartedAt        *time.Time
	CompletedAt      *time.Time
	HTTPStatus       *int
	DurationMillis   *int64
	ResponseBodyHash *string    `gorm:"type:text"`
	SafeError        *string    `gorm:"type:text"`
	NextAttemptAt    *time.Time `gorm:"index"`
}

func (WebhookAttempt) TableName() string { return "webhook_attempts" }

type IdempotencyRecord struct {
	ID                string `gorm:"type:uuid;primaryKey"`
	ClientID          string `gorm:"type:uuid;not null;uniqueIndex:idx_idempotency,priority:1"`
	Operation         string `gorm:"type:text;not null;uniqueIndex:idx_idempotency,priority:2"`
	KeyHash           string `gorm:"type:text;not null;uniqueIndex:idx_idempotency,priority:3"`
	RequestHash       string `gorm:"type:text;not null"`
	ResponseStatus    *int
	ResponseBody      []byte    `gorm:"type:jsonb"`
	CreatedResourceID *string   `gorm:"type:uuid"`
	State             string    `gorm:"type:text;not null"`
	ExpiresAt         time.Time `gorm:"not null;index"`
	CreatedAt         time.Time `gorm:"not null"`
	UpdatedAt         time.Time `gorm:"not null"`
}

func (IdempotencyRecord) TableName() string { return "idempotency_records" }

type OutboxMessage struct {
	ID            string     `gorm:"type:uuid;primaryKey"`
	Topic         string     `gorm:"type:text;not null"`
	AggregateType string     `gorm:"type:text;not null"`
	AggregateID   string     `gorm:"type:uuid;not null;index"`
	Payload       []byte     `gorm:"type:jsonb;not null"`
	CreatedAt     time.Time  `gorm:"not null"`
	AvailableAt   time.Time  `gorm:"not null;index"`
	LeaseOwner    *string    `gorm:"type:text"`
	LeaseUntil    *time.Time `gorm:"index"`
	Attempts      int        `gorm:"not null"`
	ProcessedAt   *time.Time `gorm:"index"`
	LastError     *string    `gorm:"type:text"`
}

func (OutboxMessage) TableName() string { return "outbox_messages" }

func MigrationModels() []any {
	return []any{
		&User{},
		&UserIdentity{},
		&RetailSession{},
		&APIClient{},
		&APIKey{},
		&Order{},
		&OrderEvent{},
		&PaymentCheckout{},
		&GatewayEvent{},
		&StellarTransaction{},
		&WebhookEndpoint{},
		&WebhookEvent{},
		&WebhookAttempt{},
		&IdempotencyRecord{},
		&OutboxMessage{},
	}
}
