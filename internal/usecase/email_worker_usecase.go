package usecase

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type emailDeliveryPayload struct {
	Recipient string `json:"recipient"`
	Link      string `json:"link"`
}

type encryptedEmailDeliveryPayload struct {
	Ciphertext string `json:"ciphertext"`
}

type EmailDeliveryConfig struct {
	EncryptionKey []byte
	LeaseDuration time.Duration
	RetryDelay    time.Duration
	MaxAttempts   int
	Now           func() time.Time
}

type EmailDeliveryUsecase struct {
	repository EmailOutboxRepository
	sender     EmailSender
	config     EmailDeliveryConfig
}

func NewEmailDeliveryUsecase(repository EmailOutboxRepository, sender EmailSender, config EmailDeliveryConfig) (*EmailDeliveryUsecase, error) {
	if repository == nil || sender == nil || len(config.EncryptionKey) != 32 ||
		config.LeaseDuration <= 0 || config.RetryDelay <= 0 || config.MaxAttempts <= 0 || config.Now == nil {
		return nil, errors.New("valid email delivery dependencies and configuration are required")
	}
	return &EmailDeliveryUsecase{repository: repository, sender: sender, config: config}, nil
}

func (s *EmailDeliveryUsecase) RunOnce(ctx context.Context, workerID string) (bool, error) {
	now := s.config.Now().UTC()
	job, err := s.repository.LeaseEmail(ctx, workerID, now, s.config.LeaseDuration)
	if errors.Is(err, ErrNoJob) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("leasing email: %w", err)
	}

	payload, err := decryptEmailDeliveryPayload(s.config.EncryptionKey, job.Payload)
	if err != nil {
		failErr := s.repository.FailEmail(ctx, job.ID, now, "invalid email delivery payload")
		if failErr != nil {
			return true, errors.Join(fmt.Errorf("decrypting email delivery payload: %w", err), failErr)
		}
		return true, fmt.Errorf("decrypting email delivery payload: %w", err)
	}

	var sendErr error
	switch job.Topic {
	case EmailVerificationTopic:
		sendErr = s.sender.SendEmailVerification(ctx, payload.Recipient, payload.Link)
	case PasswordResetTopic:
		sendErr = s.sender.SendPasswordReset(ctx, payload.Recipient, payload.Link)
	default:
		failErr := s.repository.FailEmail(ctx, job.ID, now, "unsupported email delivery topic")
		if failErr != nil {
			return true, errors.Join(fmt.Errorf("unsupported email delivery topic %q", job.Topic), failErr)
		}
		return true, fmt.Errorf("unsupported email delivery topic %q", job.Topic)
	}
	if sendErr != nil {
		if job.Attempts >= s.config.MaxAttempts {
			failErr := s.repository.FailEmail(ctx, job.ID, now, "email delivery failed after maximum attempts")
			if failErr != nil {
				return true, errors.Join(fmt.Errorf("sending email: %w", sendErr), failErr)
			}
			return true, fmt.Errorf("sending email after maximum attempts: %w", sendErr)
		}
		if retryErr := s.repository.RetryEmail(ctx, job.ID, now.Add(s.config.RetryDelay), "email delivery failed"); retryErr != nil {
			return true, errors.Join(fmt.Errorf("sending email: %w", sendErr), retryErr)
		}
		return true, fmt.Errorf("sending email: %w", sendErr)
	}
	if err := s.repository.CompleteEmail(ctx, job.ID, now); err != nil {
		return true, fmt.Errorf("completing email delivery: %w", err)
	}
	return true, nil
}

func (s *AuthUsecase) newEmailDelivery(topic, aggregateID, recipient, link string, now time.Time) (EmailDeliveryRecord, error) {
	payload, err := encryptEmailDeliveryPayload(s.config.TransactionEncryptionKey, emailDeliveryPayload{
		Recipient: recipient,
		Link:      link,
	})
	if err != nil {
		return EmailDeliveryRecord{}, err
	}
	jobID := newUUID()
	if jobID == "" {
		return EmailDeliveryRecord{}, errors.New("generating email job id")
	}
	return EmailDeliveryRecord{
		ID:          jobID,
		Topic:       topic,
		AggregateID: aggregateID,
		Payload:     payload,
		AvailableAt: now.UTC(),
	}, nil
}

func encryptEmailDeliveryPayload(key []byte, payload emailDeliveryPayload) ([]byte, error) {
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := cryptorand.Read(nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	return json.Marshal(encryptedEmailDeliveryPayload{
		Ciphertext: base64.RawStdEncoding.EncodeToString(append(nonce, ciphertext...)),
	})
}

func decryptEmailDeliveryPayload(key, payload []byte) (emailDeliveryPayload, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return emailDeliveryPayload{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return emailDeliveryPayload{}, errors.New("invalid encrypted email delivery payload")
	}
	var encrypted encryptedEmailDeliveryPayload
	if json.Unmarshal(payload, &encrypted) != nil || encrypted.Ciphertext == "" {
		return emailDeliveryPayload{}, errors.New("invalid encrypted email delivery payload")
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(encrypted.Ciphertext)
	if err != nil || len(ciphertext) < gcm.NonceSize() {
		return emailDeliveryPayload{}, errors.New("invalid encrypted email delivery payload")
	}
	plaintext, err := gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], nil)
	if err != nil {
		return emailDeliveryPayload{}, errors.New("invalid encrypted email delivery payload")
	}
	var result emailDeliveryPayload
	if err := json.Unmarshal(plaintext, &result); err != nil || result.Recipient == "" || result.Link == "" {
		return emailDeliveryPayload{}, errors.New("invalid email delivery payload")
	}
	return result, nil
}
