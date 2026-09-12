package config

import (
	"net/netip"
	"net/url"
	"regexp"
)

func validateOAuthConfig(c OAuthSecurityConfig) []string {
	var errs []string
	if len(c.TrustedProxyCIDRs) > 16 {
		errs = append(errs, "security.oauth.trusted_proxy_cidrs has at most 16 networks")
	}
	for _, cidr := range c.TrustedProxyCIDRs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil || prefix.Bits() == 0 || prefix.Addr().Is4In6() || prefix != prefix.Masked() {
			errs = append(errs, "security.oauth.trusted_proxy_cidrs requires explicit canonical networks, never all addresses")
		}
	}
	for name, provider := range map[string]OAuthProviderSecurityConfig{"google": c.Google, "facebook": c.Facebook} {
		if provider.ClientID == "" && provider.ClientSecret == "" && provider.RedirectURL == "" && provider.GraphVersion == "" {
			continue
		}
		u, err := url.Parse(provider.RedirectURL)
		if err != nil || provider.ClientID == "" || len(provider.ClientID) > 512 || provider.ClientSecret == "" || len(provider.ClientSecret) > 4096 || len(provider.RedirectURL) > 2048 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Path != "/api/auth/oauth/callback" || len(u.Query()) != 1 || len(u.Query()["provider"]) != 1 || u.Query().Get("provider") != name {
			errs = append(errs, "security.oauth."+name+" requires bounded client credentials and an absolute HTTPS callback with the matching provider query and no userinfo or fragment")
		}
		if name == "facebook" && !regexp.MustCompile(`^v[1-9][0-9]?\.[0-9]{1,2}$`).MatchString(provider.GraphVersion) {
			errs = append(errs, "security.oauth.facebook.graph_version must be an explicit reviewed version")
		}
	}
	return errs
}
