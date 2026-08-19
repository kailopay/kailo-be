package entity

import (
	"errors"
	"fmt"
)

const StroopsPerXLM Stroops = 10_000_000

var ErrInvalidAmount = errors.New("amount must be positive")

type IDR int64

func (amount IDR) Validate() error {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	return nil
}

type Stroops int64

func (amount Stroops) Validate() error {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	return nil
}

func (amount Stroops) String() string {
	whole := int64(amount) / int64(StroopsPerXLM)
	fraction := int64(amount) % int64(StroopsPerXLM)
	return fmt.Sprintf("%d.%07d", whole, fraction)
}
