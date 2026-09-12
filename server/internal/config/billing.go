package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/google/uuid"
	"net/mail"
	"regexp"
	"slices"
	"strings"
)

// BillingConfig is disabled unless a platform has real credentials and an exact
// server-owned product mapping. These limits bound external verification work.
type BillingConfig struct {
	MaxSubscriptionEntries int                 `yaml:"max_subscription_entries"`
	MaxReceiptBytes        int64               `yaml:"max_receipt_bytes"`
	MaxResponseBytes       int64               `yaml:"max_response_bytes"`
	HTTPTimeoutS           int                 `yaml:"http_timeout_s"`
	TaskPollIntervalS      int                 `yaml:"task_poll_interval_s"`
	TaskBatchSize          int                 `yaml:"task_batch_size"`
	MaxConcurrentRequests  int                 `yaml:"max_concurrent_requests"`
	Google                 GoogleBillingConfig `yaml:"google"`
	Apple                  AppleBillingConfig  `yaml:"apple"`
}
type BillingProduct struct {
	Kind string `yaml:"kind"`
	Noin int    `yaml:"noin"`
}
type GoogleBillingConfig struct {
	Enabled             bool                      `yaml:"enabled"`
	PackageName         string                    `yaml:"package_name"`
	ServiceAccountEmail string                    `yaml:"service_account_email"`
	PrivateKey          string                    `yaml:"private_key" json:"-"`
	AllowTestPurchases  bool                      `yaml:"allow_test_purchases"`
	Products            map[string]BillingProduct `yaml:"products"`
}
type AppleBillingConfig struct {
	Enabled     bool                      `yaml:"enabled"`
	BundleID    string                    `yaml:"bundle_id"`
	AppAppleID  int64                     `yaml:"app_apple_id"`
	IssuerID    string                    `yaml:"issuer_id"`
	KeyID       string                    `yaml:"key_id"`
	PrivateKey  string                    `yaml:"private_key" json:"-"`
	Environment string                    `yaml:"environment"`
	Products    map[string]BillingProduct `yaml:"products"`
}

func (c BillingConfig) Validate(e EconomyTuning) error {
	if errs := validateBilling(c, e); len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}
func validateBilling(c BillingConfig, e EconomyTuning) []string {
	var errs []string
	if !c.Google.Enabled && !c.Apple.Enabled {
		return errs
	}
	if c.MaxSubscriptionEntries < 1 || c.MaxSubscriptionEntries > 256 {
		errs = append(errs, "billing.max_subscription_entries must be 1..256")
	}
	if c.MaxReceiptBytes < 1024 || c.MaxReceiptBytes > 65536 {
		errs = append(errs, "billing.max_receipt_bytes must be 1024..65536")
	}
	if c.MaxResponseBytes < 1024 || c.MaxResponseBytes > 1048576 {
		errs = append(errs, "billing.max_response_bytes must be 1024..1048576")
	}
	if c.HTTPTimeoutS < 1 || c.HTTPTimeoutS > 30 {
		errs = append(errs, "billing.http_timeout_s must be 1..30")
	}
	if c.TaskPollIntervalS < 30 || c.TaskPollIntervalS > 3600 {
		errs = append(errs, "billing.task_poll_interval_s must be30..3600")
	}
	if c.TaskBatchSize < 1 || c.TaskBatchSize > 100 {
		errs = append(errs, "billing.task_batch_size must be1..100")
	}
	if c.MaxConcurrentRequests < 1 || c.MaxConcurrentRequests > 32 {
		errs = append(errs, "billing.max_concurrent_requests must be 1..32")
	}
	if c.Google.Enabled {
		if !billingAppID(c.Google.PackageName) {
			errs = append(errs, "billing.google.package_name is required")
		}
		a, err := mail.ParseAddress(c.Google.ServiceAccountEmail)
		if err != nil || a.Address != c.Google.ServiceAccountEmail {
			errs = append(errs, "billing.google.service_account_email is invalid")
		}
		if _, err := ParseBillingPrivateKey(c.Google.PrivateKey, "google_play"); err != nil {
			errs = append(errs, "billing.google.private_key must be an RSA key of at least2048bits")
		}
		errs = append(errs, validateBillingProducts("google", c.Google.Products, e)...)
	}
	if c.Apple.Enabled {
		if !billingAppID(c.Apple.BundleID) || c.Apple.AppAppleID <= 0 {
			errs = append(errs, "billing.apple app identity is required")
		}
		if c.Apple.Environment != "Production" && c.Apple.Environment != "Sandbox" {
			errs = append(errs, "billing.apple.environment must be Production or Sandbox")
		}
		issuer, issuerErr := uuid.Parse(c.Apple.IssuerID)
		if issuerErr != nil || issuer == uuid.Nil || issuer.String() != c.Apple.IssuerID || !regexp.MustCompile(`^[A-Z0-9]{10}$`).MatchString(c.Apple.KeyID) {
			errs = append(errs, "billing.apple issuer/key identity is invalid")
		}
		if _, err := ParseBillingPrivateKey(c.Apple.PrivateKey, "app_store"); err != nil {
			errs = append(errs, "billing.apple.private_key must be a P256 key")
		}
		errs = append(errs, validateBillingProducts("apple", c.Apple.Products, e)...)
	}
	return errs
}
func billingAppID(s string) bool {
	return len(s) <= 255 && regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*(\.[A-Za-z0-9][A-Za-z0-9_-]*)+$`).MatchString(s)
}
func validateBillingProducts(platform string, products map[string]BillingProduct, e EconomyTuning) []string {
	var errs []string
	if len(products) == 0 || len(products) > 100 {
		return []string{"billing." + platform + ".products must contain1..100 entries"}
	}
	for id, p := range products {
		if len(id) > 200 || !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`).MatchString(id) {
			errs = append(errs, "billing."+platform+" product identity is invalid")
		}
		switch p.Kind {
		case "noin":
			if p.Noin <= 0 || p.Noin > 1000000000 || !slices.Contains(e.NoinBundles, p.Noin) {
				errs = append(errs, "billing."+platform+" Noin product must map an existing configured bundle")
			}
		case "premium_monthly", "premium_yearly":
			if p.Noin != 0 {
				errs = append(errs, "billing."+platform+" subscription cannot grant Noin")
			}
		default:
			errs = append(errs, "billing."+platform+" product kind is invalid")
		}
	}
	return errs
}

// ParseBillingPrivateKey validates private credentials without returning their
// content in errors. Callers use only the platform-specific signing algorithm.
func ParseBillingPrivateKey(value, platform string) (any, error) {
	invalid := fmt.Errorf("invalid billing private key")
	if len(value) > 16384 {
		return nil, invalid
	}
	block, rest := pem.Decode([]byte(value))
	if block == nil || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, invalid
	}
	var key any
	var err error
	switch block.Type {
	case "PRIVATE KEY":
		key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		key, err = x509.ParseECPrivateKey(block.Bytes)
	default:
		return nil, invalid
	}
	if err != nil {
		return nil, invalid
	}
	if platform == "google_play" {
		if k, ok := key.(*rsa.PrivateKey); ok && k.N.BitLen() >= 2048 {
			return k, nil
		}
	}
	if platform == "app_store" {
		if k, ok := key.(*ecdsa.PrivateKey); ok && k.Curve == elliptic.P256() {
			return k, nil
		}
	}
	return nil, invalid
}
