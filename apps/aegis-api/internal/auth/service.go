package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Claims struct {
	TokenType string `json:"token_type"`
	jwt.RegisteredClaims
}

type Service struct {
	users      repositories.UserRepository
	issuer     string
	audience   string
	key        []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

func NewService(users repositories.UserRepository, issuer, audience string, key []byte, accessTTL, refreshTTL time.Duration) (*Service, error) {
	if len(key) < 32 {
		return nil, errors.New("JWT signing key must be at least 32 bytes")
	}
	return &Service{users: users, issuer: issuer, audience: audience, key: key, accessTTL: accessTTL, refreshTTL: refreshTTL}, nil
}

func HashPassword(password string) ([]byte, error) {
	if len(password) < 12 {
		return nil, errors.New("password must contain at least 12 characters")
	}
	return bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
}

func (s *Service) Login(ctx context.Context, username, password string) (Tokens, domain.Principal, error) {
	user, err := s.users.GetUserByUsername(ctx, username)
	if err != nil || !user.Enabled || bcrypt.CompareHashAndPassword(user.PasswordHash, []byte(password)) != nil {
		return Tokens{}, domain.Principal{}, domain.ErrUnauthorized
	}
	principal, err := s.users.GetPrincipal(ctx, user.ID)
	if err != nil {
		return Tokens{}, domain.Principal{}, err
	}
	tokens, err := s.issue(user.ID)
	return tokens, principal, err
}

func (s *Service) Refresh(ctx context.Context, raw string) (Tokens, error) {
	claims, err := s.parse(raw, "refresh")
	if err != nil {
		return Tokens{}, domain.ErrUnauthorized
	}
	user, err := s.users.GetUser(ctx, claims.Subject)
	if err != nil || !user.Enabled {
		return Tokens{}, domain.ErrUnauthorized
	}
	return s.issue(user.ID)
}

func (s *Service) Authenticate(ctx context.Context, raw string) (domain.Principal, error) {
	claims, err := s.parse(raw, "access")
	if err != nil {
		return domain.Principal{}, domain.ErrUnauthorized
	}
	principal, err := s.users.GetPrincipal(ctx, claims.Subject)
	if err != nil {
		return domain.Principal{}, domain.ErrUnauthorized
	}
	return principal, nil
}

func (s *Service) issue(subject string) (Tokens, error) {
	now := time.Now().UTC()
	access, err := s.sign(subject, "access", now, now.Add(s.accessTTL))
	if err != nil {
		return Tokens{}, err
	}
	refresh, err := s.sign(subject, "refresh", now, now.Add(s.refreshTTL))
	if err != nil {
		return Tokens{}, err
	}
	return Tokens{AccessToken: access, RefreshToken: refresh, TokenType: "Bearer", ExpiresIn: int64(s.accessTTL.Seconds())}, nil
}

func (s *Service) sign(subject, tokenType string, issuedAt, expiresAt time.Time) (string, error) {
	claims := Claims{TokenType: tokenType, RegisteredClaims: jwt.RegisteredClaims{Issuer: s.issuer, Subject: subject, Audience: jwt.ClaimStrings{s.audience}, IssuedAt: jwt.NewNumericDate(issuedAt), NotBefore: jwt.NewNumericDate(issuedAt), ExpiresAt: jwt.NewNumericDate(expiresAt)}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.key)
}

func (s *Service) parse(raw, expectedType string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(raw, &Claims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method %s", token.Method.Alg())
		}
		return s.key, nil
	}, jwt.WithIssuer(s.issuer), jwt.WithAudience(s.audience), jwt.WithExpirationRequired(), jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !token.Valid {
		return nil, domain.ErrUnauthorized
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || claims.TokenType != expectedType || claims.Subject == "" {
		return nil, domain.ErrUnauthorized
	}
	return claims, nil
}
