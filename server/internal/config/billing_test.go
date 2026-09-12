package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
)

func TestBillingConfigurationFailsClosed(t *testing.T) {
	if errs := validateBilling(BillingConfig{}, EconomyTuning{}); len(errs) != 0 {
		t.Fatal(errs)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	base := BillingConfig{MaxSubscriptionEntries: 64, MaxReceiptBytes: 16384, MaxResponseBytes: 262144, HTTPTimeoutS: 10, TaskPollIntervalS: 60, TaskBatchSize: 20, MaxConcurrentRequests: 4,
		Google: GoogleBillingConfig{Enabled: true, PackageName: "example.knowoff", ServiceAccountEmail: "fixture@example.iam.gserviceaccount.com", PrivateKey: keyPEM, Products: map[string]BillingProduct{"coins": {Kind: "noin", Noin: 500}}}}
	if errs := validateBilling(base, EconomyTuning{NoinBundles: []int{500}}); len(errs) != 0 {
		t.Fatal(errs)
	}
	invalid := base
	invalid.Google.PrivateKey = "not-a-key"
	if errs := validateBilling(invalid, EconomyTuning{NoinBundles: []int{500}}); len(errs) == 0 {
		t.Fatal("invalid private key accepted")
	}
	for _, mutate := range []func(*BillingConfig){
		func(c *BillingConfig) { c.MaxSubscriptionEntries = 0 }, func(c *BillingConfig) { c.MaxSubscriptionEntries = 257 }, func(c *BillingConfig) { c.MaxConcurrentRequests = 0 }, func(c *BillingConfig) { c.MaxConcurrentRequests = 33 }, func(c *BillingConfig) { c.TaskPollIntervalS = 0 }, func(c *BillingConfig) { c.TaskBatchSize = 101 }, func(c *BillingConfig) { c.MaxReceiptBytes = -1 }, func(c *BillingConfig) { c.MaxResponseBytes = 0 }, func(c *BillingConfig) { c.HTTPTimeoutS = 0 },
		func(c *BillingConfig) { c.Google.PackageName = "" }, func(c *BillingConfig) { c.Google.ServiceAccountEmail = "" },
		func(c *BillingConfig) {
			c.Google.Products = map[string]BillingProduct{"coins": {Kind: "noin", Noin: 999}}
		},
		func(c *BillingConfig) {
			c.Google.Products = map[string]BillingProduct{"sub": {Kind: "premium_monthly", Noin: 500}}
		},
	} {
		cfg := base
		mutate(&cfg)
		if errs := validateBilling(cfg, EconomyTuning{NoinBundles: []int{500}}); len(errs) == 0 {
			t.Fatal("invalid billing config accepted")
		}
	}
	// Validation never echoes the credential itself into startup logs.
	for _, err := range validateBilling(invalid, EconomyTuning{NoinBundles: []int{500}}) {
		if strings.Contains(err, "not-a-key") {
			t.Fatal("credential echoed")
		}
	}
}
