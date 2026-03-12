package http

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type OIDCAuth struct {
	audience         string
	clientID         string
	clientSecret     string
	introspectionURL string
	oauth2           oauth2.Config
}

func NewOIDCAuth(ctx context.Context, issuer, audience, clientID, clientSecret, redirectURL string) (*OIDCAuth, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("initialize oidc provider: %w", err)
	}

	var metadata struct {
		IntrospectionEndpoint string `json:"introspection_endpoint"`
	}
	if err := provider.Claims(&metadata); err != nil {
		return nil, fmt.Errorf("read oidc provider metadata: %w", err)
	}
	if metadata.IntrospectionEndpoint == "" {
		return nil, fmt.Errorf("oidc issuer metadata does not expose introspection_endpoint")
	}

	return &OIDCAuth{
		audience:         audience,
		clientID:         clientID,
		clientSecret:     clientSecret,
		introspectionURL: metadata.IntrospectionEndpoint,
		oauth2: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
	}, nil
}

func (a *OIDCAuth) VerifyAccessToken(ctx context.Context, rawToken string) error {
	body := url.Values{}
	body.Set("token", rawToken)
	body.Set("token_type_hint", "access_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.introspectionURL, strings.NewReader(body.Encode()))
	if err != nil {
		log.Printf("oidc verify: failed to build introspection request: %v", err)
		return fmt.Errorf("create token introspection request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(a.clientID, a.clientSecret)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("oidc verify: introspection request failed: %v", err)
		return fmt.Errorf("send token introspection request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		log.Printf("oidc verify: introspection status=%d body=%q", res.StatusCode, string(payload))
		return fmt.Errorf("token introspection failed with status %d: %s", res.StatusCode, string(payload))
	}

	var reply struct {
		Active   bool `json:"active"`
		Audience any  `json:"aud"`
	}
	if err := json.NewDecoder(res.Body).Decode(&reply); err != nil {
		log.Printf("oidc verify: failed to decode introspection response: %v", err)
		return fmt.Errorf("decode token introspection response: %w", err)
	}
	if !reply.Active {
		log.Printf("oidc verify: token is inactive")
		return fmt.Errorf("token is not active")
	}

	return nil
}

func (a *OIDCAuth) AuthCodeURL(state, codeChallenge string) string {
	return a.oauth2.AuthCodeURL(
		state,
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("audience", a.audience),
	)
}

func (a *OIDCAuth) Exchange(ctx context.Context, code, codeVerifier string) (*oauth2.Token, error) {
	return a.oauth2.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", codeVerifier))
}

func claimHasAudience(audClaim any, required string) bool {
	switch aud := audClaim.(type) {
	case string:
		return aud == required
	case []string:
		for _, entry := range aud {
			if entry == required {
				return true
			}
		}
	case []any:
		for _, entry := range aud {
			if v, ok := entry.(string); ok && v == required {
				return true
			}
		}
	}
	return false
}

func randomBase64URL(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func codeChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
