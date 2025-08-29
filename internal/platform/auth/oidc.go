package auth

import (
	"context"
	"errors"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/log"
	"github.com/coreos/go-oidc/v3/oidc"
	"net/http"
	"strings"
)

type OIDCConfig struct {
	Issuer        string
	Audiences     []string
	RequiredScope string
	Logger        *log.Logger
}

type OIDC struct {
	verifiers     []*oidc.IDTokenVerifier
	requiredScope string
	log           *log.Logger
}

func NewOIDC(ctx context.Context, cfg OIDCConfig) (*OIDC, error) {
	if cfg.Issuer == "" || len(cfg.Audiences) == 0 {
		return nil, errors.New("missing issuer/audience")
	}
	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, err
	}
	var verifiers []*oidc.IDTokenVerifier
	for _, aud := range cfg.Audiences {
		verifiers = append(verifiers, provider.Verifier(&oidc.Config{
			ClientID:          aud,
			SkipClientIDCheck: false,
			SkipIssuerCheck:   false,
		}))
	}

	return &OIDC{verifiers: verifiers, requiredScope: cfg.RequiredScope, log: cfg.Logger}, nil
}

func (m *OIDC) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := bearer(r.Header.Get("Authorization"))
		if raw == "" {
			m.log.Error("missing bearer token")
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		ok := false
		claims := map[string]any{}
		for _, v := range m.verifiers {
			if idt, err := v.Verify(r.Context(), raw); err == nil {
				if err := idt.Claims(&claims); err != nil {
					m.log.Error("failed to parse claims: %v", log.Err(err))
					http.Error(w, "invalid token", http.StatusUnauthorized)
					return
				}
				ok = true
				break
			}
		}
		if !ok {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		if m.requiredScope != "" && !hasScope(claims, m.requiredScope) {
			http.Error(w, "insufficient scope", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearer(h string) string {
	if !strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return ""
	}

	return strings.TrimSpace(h[7:])
}

func hasScope(claims map[string]any, want string) bool {
        if v, ok := claims["scope"].(string); ok {
                for _, s := range strings.Split(v, " ") {
                        if s == want {
                                return true
                        }
                }
        }
	if arr, ok := claims["scp"].([]any); ok {
		for _, s := range arr {
			if sstr, ok := s.(string); ok && sstr == want {
				return true
			}
		}
	}

	return false
}
