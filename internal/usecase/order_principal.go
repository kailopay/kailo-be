package usecase

import (
	"errors"
	"strings"
)

// OrderPrincipalKind identifies the credential scope that owns an order.
// Keeping this explicit prevents a user ID from being accidentally queried as
// an API client ID (or the reverse).
type OrderPrincipalKind string

const (
	OrderPrincipalAPIClient     OrderPrincipalKind = "api_client"
	OrderPrincipalRetailSession OrderPrincipalKind = "retail_session"
)

var ErrInvalidOrderPrincipal = errors.New("invalid order principal")

// OrderPrincipal is the authenticated owner context for order operations.
// Exactly one owner kind must be populated.
type OrderPrincipal struct {
	Kind        OrderPrincipalKind
	ClientID    string
	OwnerUserID string
	SessionID   string
}

func NewAPIClientOrderPrincipal(principal Principal) (OrderPrincipal, error) {
	orderPrincipal := OrderPrincipal{
		Kind:        OrderPrincipalAPIClient,
		ClientID:    strings.TrimSpace(principal.ClientID),
		OwnerUserID: strings.TrimSpace(principal.OwnerUserID),
	}
	if err := orderPrincipal.Validate(); err != nil {
		return OrderPrincipal{}, err
	}
	return orderPrincipal, nil
}

func NewRetailSessionOrderPrincipal(user AuthenticatedUser) (OrderPrincipal, error) {
	orderPrincipal := OrderPrincipal{
		Kind:        OrderPrincipalRetailSession,
		OwnerUserID: strings.TrimSpace(user.User.ID),
		SessionID:   strings.TrimSpace(user.SessionID),
	}
	if !user.User.EmailVerified {
		return OrderPrincipal{}, ErrInvalidOrderPrincipal
	}
	if err := orderPrincipal.Validate(); err != nil {
		return OrderPrincipal{}, err
	}
	return orderPrincipal, nil
}

func (p OrderPrincipal) Validate() error {
	switch p.Kind {
	case OrderPrincipalAPIClient:
		if p.ClientID == "" || p.OwnerUserID == "" || p.SessionID != "" {
			return ErrInvalidOrderPrincipal
		}
	case OrderPrincipalRetailSession:
		if p.OwnerUserID == "" || p.SessionID == "" || p.ClientID != "" {
			return ErrInvalidOrderPrincipal
		}
	default:
		return ErrInvalidOrderPrincipal
	}
	return nil
}

func (p OrderPrincipal) IsAPIClient() bool {
	return p.Kind == OrderPrincipalAPIClient
}

func (p OrderPrincipal) IsRetailSession() bool {
	return p.Kind == OrderPrincipalRetailSession
}
