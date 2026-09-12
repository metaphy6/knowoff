package config

import (
	"strings"
	"testing"
)

func TestOAuthConfigRequiresCompleteBoundedPinnedProviders(t *testing.T) {
	for _, provider := range []string{"google", "facebook"} {
		valid := OAuthProviderSecurityConfig{ClientID: "fixture-client", ClientSecret: "fixture-secret", RedirectURL: "https://knowoff.invalid/api/auth/oauth/callback?provider=" + provider, GraphVersion: "v21.0"}
		for _, test := range []struct {
			name   string
			change func(*OAuthProviderSecurityConfig)
		}{
			{"missing_secret", func(c *OAuthProviderSecurityConfig) { c.ClientSecret = "" }},
			{"missing_callback", func(c *OAuthProviderSecurityConfig) { c.RedirectURL = "" }},
			{"http", func(c *OAuthProviderSecurityConfig) {
				c.RedirectURL = "http://knowoff.invalid/api/auth/oauth/callback?provider=" + provider
			}},
			{"wrong_provider", func(c *OAuthProviderSecurityConfig) {
				c.RedirectURL = "https://knowoff.invalid/api/auth/oauth/callback?provider=other"
			}},
			{"credentials", func(c *OAuthProviderSecurityConfig) {
				c.RedirectURL = "https://secret@knowoff.invalid/api/auth/oauth/callback?provider=" + provider
			}},
			{"wrong_path", func(c *OAuthProviderSecurityConfig) {
				c.RedirectURL = "https://knowoff.invalid/other?provider=" + provider
			}},
			{"duplicate_provider", func(c *OAuthProviderSecurityConfig) { c.RedirectURL += "&provider=" + provider }},
			{"fragment", func(c *OAuthProviderSecurityConfig) { c.RedirectURL += "#token" }},
			{"oversized_id", func(c *OAuthProviderSecurityConfig) { c.ClientID = strings.Repeat("s", 513) }},
		} {
			t.Run(provider+"/"+test.name, func(t *testing.T) {
				c := valid
				test.change(&c)
				set := OAuthSecurityConfig{}
				if provider == "google" {
					set.Google = c
				} else {
					set.Facebook = c
				}
				if len(validateOAuthConfig(set)) == 0 {
					t.Fatal("invalid provider config accepted")
				}
			})
		}
		set := OAuthSecurityConfig{}
		if provider == "google" {
			set.Google = valid
		} else {
			set.Facebook = valid
		}
		if errs := validateOAuthConfig(set); len(errs) > 0 {
			t.Fatal(errs)
		}
	}
	for _, version := range []string{"", "latest", "v21.0/other", "v1"} {
		if errs := validateOAuthConfig(OAuthSecurityConfig{Facebook: OAuthProviderSecurityConfig{ClientID: "fixture", ClientSecret: "fixture", RedirectURL: "https://knowoff.invalid/api/auth/oauth/callback?provider=facebook", GraphVersion: version}}); len(errs) == 0 {
			t.Fatal("unbounded Facebook version accepted")
		}
	}
	if errs := validateOAuthConfig(OAuthSecurityConfig{}); len(errs) != 0 {
		t.Fatal("disabled defaults invalid", errs)
	}
}

func TestOAuthTrustedProxyConfigurationIsExplicitAndBounded(t *testing.T) {
	for _, cidr := range []string{"0.0.0.0/0", "::/0", "not-a-network", "10.0.0.9/24", "::ffff:192.0.2.0/120"} {
		if errs := validateOAuthConfig(OAuthSecurityConfig{TrustedProxyCIDRs: []string{cidr}}); len(errs) == 0 {
			t.Fatal("unsafe proxy trust accepted", cidr)
		}
	}
	if errs := validateOAuthConfig(OAuthSecurityConfig{TrustedProxyCIDRs: []string{"10.0.0.0/24", "2001:db8::/64"}}); len(errs) != 0 {
		t.Fatal(errs)
	}
	if errs := validateOAuthConfig(OAuthSecurityConfig{TrustedProxyCIDRs: make([]string, 17)}); len(errs) == 0 {
		t.Fatal("unbounded proxy inventory")
	}
}
