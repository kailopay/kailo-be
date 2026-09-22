package usecase

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	WebhookDeliveryDelivered            = "delivered"
	WebhookDeliveryRetryScheduled       = "retry_scheduled"
	WebhookDeliveryFailed               = "failed"
	WebhookDeliveryExhausted            = "exhausted"
	webhookResponseBodyLimit      int64 = 64 << 10
)

// WebhookDNSResolver is deliberately small so delivery policy can be tested
// without making network calls.
type WebhookDNSResolver interface {
	LookupIP(ctx context.Context, host string) ([]net.IP, error)
}

type systemWebhookDNSResolver struct{}

func (systemWebhookDNSResolver) LookupIP(ctx context.Context, host string) ([]net.IP, error) {
	return net.DefaultResolver.LookupIP(ctx, "ip", host)
}

func NewWebhookDNSResolver() WebhookDNSResolver { return systemWebhookDNSResolver{} }

// WebhookHTTPClient is the only network seam used by webhook delivery.
type WebhookHTTPClient interface {
	Do(request *http.Request) (*http.Response, error)
}

// WebhookSecretProtector protects endpoint signing secrets at rest. A hash is
// insufficient because the worker must recover the secret to sign a payload.
type WebhookSecretProtector interface {
	Protect(secret string) ([]byte, error)
	Unprotect(protected []byte) (string, error)
}

// WebhookSecretBox uses an authenticated encryption key supplied by the
// application secret configuration. The protected bytes are safe to encode in
// the existing secret_reference column.
type WebhookSecretBox struct {
	aead cipher.AEAD
}

func NewWebhookSecretBox(key []byte) (*WebhookSecretBox, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating webhook secret cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating webhook secret cipher mode: %w", err)
	}
	return &WebhookSecretBox{aead: aead}, nil
}

func (b *WebhookSecretBox) Protect(secret string) ([]byte, error) {
	if b == nil || b.aead == nil || secret == "" {
		return nil, errors.New("webhook secret protector is not initialized")
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generating webhook secret nonce: %w", err)
	}
	return b.aead.Seal(nonce, nonce, []byte(secret), nil), nil
}

func (b *WebhookSecretBox) Unprotect(protected []byte) (string, error) {
	if b == nil || b.aead == nil {
		return "", errors.New("webhook secret protector is not initialized")
	}
	nonceSize := b.aead.NonceSize()
	if len(protected) <= nonceSize {
		return "", errors.New("webhook secret reference is invalid")
	}
	secret, err := b.aead.Open(nil, protected[:nonceSize], protected[nonceSize:], nil)
	if err != nil {
		return "", errors.New("webhook secret reference cannot be opened")
	}
	return string(secret), nil
}

// BuildWebhookSignature signs the exact bytes sent over the wire. Consumers
// can verify the hex value using HMAC-SHA256 over "timestamp.payload".
func BuildWebhookSignature(eventID string, timestamp time.Time, payload, secret []byte) string {
	unix := timestamp.UTC().Unix()
	mac := hmac.New(sha256.New, secret)
	_ = eventID // The stable event ID is carried in the raw envelope and header.
	_, _ = mac.Write([]byte(strconv.FormatInt(unix, 10) + "."))
	_, _ = mac.Write(payload)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

// ValidateWebhookDestination resolves the host at delivery time and rejects
// every private, local, link-local, multicast, unspecified, or reserved
// address. All resolved answers must be public so a mixed DNS response cannot
// smuggle a private destination through the policy.
func ValidateWebhookDestination(ctx context.Context, rawURL string, resolver WebhookDNSResolver) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Hostname() == "" {
		return ErrInvalidWebhookURL
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") ||
		host == "metadata.google.internal" || strings.HasSuffix(host, ".internal") {
		return ErrInvalidWebhookURL
	}
	if resolver == nil {
		resolver = systemWebhookDNSResolver{}
	}
	var ips []net.IP
	if literal := net.ParseIP(host); literal != nil {
		ips = []net.IP{literal}
	} else {
		ips, err = resolver.LookupIP(ctx, host)
		if err != nil {
			return fmt.Errorf("resolving webhook destination: %w", err)
		}
	}
	if len(ips) == 0 {
		return ErrInvalidWebhookURL
	}
	for _, ip := range ips {
		if isBlockedWebhookIP(ip) {
			return ErrInvalidWebhookURL
		}
	}
	return nil
}

func isBlockedWebhookIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	blocked := []string{
		"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15",
		"198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "::/128", "fc00::/7",
	}
	for _, cidr := range blocked {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

// WebhookEventRecord is the immutable payload selected for delivery.
type WebhookEventRecord struct {
	ID         string
	EventType  string
	APIVersion string
	Payload    []byte
}

type WebhookDeliveryEndpoint struct {
	ID              string
	URL             string
	SecretReference []byte
}

type WebhookDeliveryAttempt struct {
	ID            string
	EventID       string
	EndpointID    string
	AttemptNumber int
}

type WebhookDeliveryJob struct {
	OutboxID string
	EventID  string
	WorkerID string
}

type WebhookDeliveryResult struct {
	Status           string
	HTTPStatus       *int
	DurationMillis   int64
	ResponseBodyHash *string
	SafeError        *string
	NextAttemptAt    *time.Time
}

// WebhookDeliveryRepository owns the transaction and lease implementation.
// The use case never holds a database transaction while making an HTTP call.
type WebhookDeliveryRepository interface {
	LoadEvent(ctx context.Context, eventID string) (WebhookEventRecord, error)
	ListDeliveryEndpoints(ctx context.Context, eventID, eventType string) ([]WebhookDeliveryEndpoint, error)
	StartAttempt(ctx context.Context, eventID, endpointID, workerID string, now time.Time, leaseDuration time.Duration, maxAttempts int) (WebhookDeliveryAttempt, bool, error)
	CompleteAttempt(ctx context.Context, attemptID string, result WebhookDeliveryResult) error
	FinalizeEvent(ctx context.Context, outboxID, eventID string, now time.Time) error
}

type WebhookDeliveryConfig struct {
	Timeout       time.Duration
	LeaseDuration time.Duration
	RetryDelay    time.Duration
	MaxAttempts   int
	Now           func() time.Time
	Resolver      WebhookDNSResolver
	Client        WebhookHTTPClient
}

type WebhookDeliveryUsecase struct {
	repository WebhookDeliveryRepository
	protector  WebhookSecretProtector
	config     WebhookDeliveryConfig
}

func NewWebhookDeliveryUsecase(repository WebhookDeliveryRepository, protector WebhookSecretProtector, config WebhookDeliveryConfig) (*WebhookDeliveryUsecase, error) {
	if repository == nil || protector == nil || config.Timeout <= 0 || config.LeaseDuration <= 0 ||
		config.RetryDelay <= 0 || config.MaxAttempts <= 0 || config.Now == nil {
		return nil, errors.New("valid webhook delivery dependencies are required")
	}
	if config.Resolver == nil {
		config.Resolver = systemWebhookDNSResolver{}
	}
	if config.Client == nil {
		config.Client = NewSafeWebhookHTTPClient(config.Timeout, config.Resolver)
	}
	return &WebhookDeliveryUsecase{repository: repository, protector: protector, config: config}, nil
}

func (s *WebhookDeliveryUsecase) RunOnce(ctx context.Context, job WebhookDeliveryJob) error {
	if job.OutboxID == "" || job.EventID == "" || job.WorkerID == "" {
		return errors.New("webhook delivery job is incomplete")
	}
	event, err := s.repository.LoadEvent(ctx, job.EventID)
	if err != nil {
		return fmt.Errorf("loading webhook event: %w", err)
	}
	endpoints, err := s.repository.ListDeliveryEndpoints(ctx, event.ID, event.EventType)
	if err != nil {
		return fmt.Errorf("listing webhook endpoints: %w", err)
	}
	now := s.config.Now().UTC()
	for _, endpoint := range endpoints {
		attempt, started, err := s.repository.StartAttempt(ctx, event.ID, endpoint.ID, job.WorkerID, now,
			s.config.LeaseDuration, s.config.MaxAttempts)
		if err != nil {
			return fmt.Errorf("starting webhook attempt: %w", err)
		}
		if !started {
			continue
		}
		result := s.deliver(ctx, event, endpoint, attempt)
		if err := s.repository.CompleteAttempt(ctx, attempt.ID, result); err != nil {
			return fmt.Errorf("completing webhook attempt: %w", err)
		}
	}
	if err := s.repository.FinalizeEvent(ctx, job.OutboxID, event.ID, s.config.Now().UTC()); err != nil {
		return fmt.Errorf("finalizing webhook event: %w", err)
	}
	return nil
}

func (s *WebhookDeliveryUsecase) deliver(ctx context.Context, event WebhookEventRecord, endpoint WebhookDeliveryEndpoint, attempt WebhookDeliveryAttempt) WebhookDeliveryResult {
	started := time.Now()
	if err := ValidateWebhookDestination(ctx, endpoint.URL, s.config.Resolver); err != nil {
		return terminalWebhookResult(WebhookDeliveryFailed, "webhook destination rejected by policy", started)
	}
	secret, err := s.protector.Unprotect(endpoint.SecretReference)
	if err != nil || secret == "" {
		return terminalWebhookResult(WebhookDeliveryFailed, "webhook signing secret is unavailable", started)
	}
	requestContext, cancel := context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodPost, endpoint.URL, strings.NewReader(string(event.Payload)))
	if err != nil {
		return terminalWebhookResult(WebhookDeliveryFailed, "webhook request is invalid", started)
	}
	eventID := publicWebhookEventID(event.ID)
	timestamp := s.config.Now().UTC()
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "KailoPay-Webhook/"+WebhookAPIVersion)
	request.Header.Set("KailoPay-Event-Id", eventID)
	request.Header.Set("KailoPay-Timestamp", strconv.FormatInt(timestamp.Unix(), 10))
	request.Header.Set("KailoPay-Signature", BuildWebhookSignature(eventID, timestamp, event.Payload, []byte(secret)))
	request.Header.Set("KailoPay-Event-Type", event.EventType)
	request.Header.Set("KailoPay-Api-Version", event.APIVersion)
	response, err := s.config.Client.Do(request)
	if err != nil {
		return s.withDuration(ClassifyWebhookDelivery(0, err, attempt.AttemptNumber, s.config.MaxAttempts, timestamp, s.config.RetryDelay), started)
	}
	if response == nil {
		return s.withDuration(ClassifyWebhookDelivery(0, errors.New("empty webhook response"), attempt.AttemptNumber, s.config.MaxAttempts, timestamp, s.config.RetryDelay), started)
	}
	var body []byte
	var readErr error
	if response.Body != nil {
		defer response.Body.Close()
		body, readErr = io.ReadAll(io.LimitReader(response.Body, webhookResponseBodyLimit))
	}
	result := ClassifyWebhookDelivery(response.StatusCode, readErr, attempt.AttemptNumber, s.config.MaxAttempts, timestamp, s.config.RetryDelay)
	hash := sha256.Sum256(body)
	hashValue := base64.RawStdEncoding.EncodeToString(hash[:])
	result.ResponseBodyHash = &hashValue
	return s.withDuration(result, started)
}

func (s *WebhookDeliveryUsecase) withDuration(result WebhookDeliveryResult, started time.Time) WebhookDeliveryResult {
	result.DurationMillis = time.Since(started).Milliseconds()
	return result
}

func terminalWebhookResult(status, message string, started time.Time) WebhookDeliveryResult {
	return WebhookDeliveryResult{Status: status, DurationMillis: time.Since(started).Milliseconds(), SafeError: webhookStringPointer(message)}
}

func publicWebhookEventID(id string) string {
	if strings.HasPrefix(id, "evt_") {
		return id
	}
	return "evt_" + id
}

func webhookStringPointer(value string) *string { return &value }

// ClassifyWebhookDelivery maps a response or network failure into the durable
// state machine. Only timeouts, 408, 429, and 5xx responses are retried.
func ClassifyWebhookDelivery(status int, networkErr error, attempt, maxAttempts int, now time.Time, retryDelay time.Duration) WebhookDeliveryResult {
	result := WebhookDeliveryResult{}
	if status > 0 {
		result.HTTPStatus = &status
	}
	if networkErr == nil && status >= http.StatusOK && status < http.StatusMultipleChoices {
		result.Status = WebhookDeliveryDelivered
		return result
	}
	retryable := networkErr != nil || status == http.StatusRequestTimeout || status == http.StatusConflict ||
		status == http.StatusTooEarly || status == http.StatusTooManyRequests || status >= 500
	if !retryable {
		result.Status = WebhookDeliveryFailed
		result.SafeError = webhookStringPointer("webhook endpoint returned a terminal HTTP response")
		return result
	}
	if attempt >= maxAttempts {
		result.Status = WebhookDeliveryExhausted
		result.SafeError = webhookStringPointer("webhook delivery retry limit reached")
		return result
	}
	result.Status = WebhookDeliveryRetryScheduled
	result.SafeError = webhookStringPointer("webhook delivery will be retried")
	if retryDelay <= 0 {
		retryDelay = time.Second
	}
	shift := attempt - 1
	if shift < 0 {
		shift = 0
	}
	if shift > 6 {
		shift = 6
	}
	delay := retryDelay * time.Duration(1<<shift)
	if delay > time.Hour {
		delay = time.Hour
	}
	next := now.Add(delay)
	result.NextAttemptAt = &next
	return result
}

// NewSafeWebhookHTTPClient disables redirects and pins each connection to a
// freshly resolved public address, preventing a redirect or DNS rebinding from
// turning a public endpoint into an internal request.
func NewSafeWebhookHTTPClient(timeout time.Duration, resolver WebhookDNSResolver) WebhookHTTPClient {
	if resolver == nil {
		resolver = systemWebhookDNSResolver{}
	}
	return &safeWebhookHTTPClient{base: &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}, resolver: resolver}
}

type safeWebhookHTTPClient struct {
	base     *http.Client
	resolver WebhookDNSResolver
}

func (c *safeWebhookHTTPClient) Do(request *http.Request) (*http.Response, error) {
	if err := ValidateWebhookDestination(request.Context(), request.URL.String(), c.resolver); err != nil {
		return nil, err
	}
	host := request.URL.Hostname()
	ips, err := c.resolver.LookupIP(request.Context(), host)
	if literal := net.ParseIP(host); literal != nil {
		ips = []net.IP{literal}
	}
	if err != nil || len(ips) == 0 {
		return nil, ErrInvalidWebhookURL
	}
	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("webhook HTTP transport is not supported")
	}
	transport := baseTransport.Clone()
	dialer := &net.Dialer{Timeout: c.base.Timeout}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		port := request.URL.Port()
		if port == "" {
			port = "443"
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}
	client := *c.base
	client.Transport = transport
	response, err := client.Do(request)
	transport.CloseIdleConnections()
	return response, err
}
