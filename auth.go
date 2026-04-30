package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	auth0ClientID    = "zE51Ep7FttlmtQV6ZEGyJKsY2jD1EtAu"
	auth0Audience    = "wework"
	auth0Scope       = "openid profile email offline_access"
	auth0RedirectURI = "https://members.wework.com/workplaceone/api/auth0/v2/callback?domain=members.wework.com/workplaceone"
	auth0ClientInfo  = "eyJuYW1lIjoiQGF1dGgwL2F1dGgwLWFuZ3VsYXIiLCJ2ZXJzaW9uIjoiMi4yLjMifQ=="
)

type WeWorkAuthenticator struct {
	email    string
	password string

	mu           sync.Mutex
	accessToken  string
	refreshToken string
	expiresAt    time.Time
}

type authTokenResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

func NewWeWorkAuthenticator(email, password string) *WeWorkAuthenticator {
	return &WeWorkAuthenticator{email: email, password: password}
}

func (a *WeWorkAuthenticator) BearerToken(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.accessToken != "" && time.Now().Before(a.expiresAt.Add(-1*time.Minute)) {
		return a.accessToken, nil
	}

	if a.refreshToken != "" {
		token, err := a.refresh(ctx)
		if err == nil {
			return token, nil
		}

		log.Println("refreshing WeWork token failed, logging in again:", err)
	}

	return a.login(ctx)
}

func (a *WeWorkAuthenticator) refresh(ctx context.Context) (string, error) {
	values := url.Values{}
	values.Set("client_id", auth0ClientID)
	values.Set("display", "")
	values.Set("prompt", "")
	values.Set("redirect_uri", auth0RedirectURI)
	values.Set("grant_type", "refresh_token")
	values.Set("refresh_token", a.refreshToken)

	token, err := postToken(ctx, http.DefaultClient, values)
	if err != nil {
		return "", err
	}

	a.storeToken(token)
	return a.accessToken, nil
}

func (a *WeWorkAuthenticator) login(ctx context.Context) (string, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	verifier, challenge, err := newPKCEPair()
	if err != nil {
		return "", err
	}

	state, err := randomURLString(48)
	if err != nil {
		return "", err
	}

	nonce, err := randomURLString(48)
	if err != nil {
		return "", err
	}

	loginURL, err := authorizeURL(state, nonce, challenge, a.email)
	if err != nil {
		return "", err
	}

	currentURL, body, err := getFollow(ctx, client, loginURL, func(u *url.URL) bool {
		return u.Host == "idp.wework.com" && u.Path == "/u/login/password"
	})
	if err != nil {
		return "", err
	}

	loginState := currentURL.Query().Get("state")
	if loginState == "" {
		loginState = extractInputValue(body, "state")
	}
	if loginState == "" {
		return "", errors.New("could not find auth0 login state")
	}

	nextURL, _, err := postFormFollow(ctx, client, currentURL.String(), url.Values{
		"state":    {loginState},
		"username": {a.email},
		"password": {a.password},
		"action":   {"default"},
	}, func(u *url.URL) bool {
		return u.Host == "idp.wework.com" && u.Path == "/u/mfa-detect-browser-capabilities"
	})
	if err != nil {
		return "", err
	}

	mfaState := nextURL.Query().Get("state")
	if mfaState == "" {
		return "", errors.New("could not find auth0 MFA capability state")
	}

	callbackURL, _, err := postFormFollow(ctx, client, nextURL.String(), url.Values{
		"state":                       {mfaState},
		"action":                      {"default"},
		"js-available":                {"true"},
		"webauthn-available":          {"true"},
		"is-brave":                    {"false"},
		"webauthn-platform-available": {"false"},
	}, func(u *url.URL) bool {
		return u.Host == "members.wework.com" && u.Path == "/workplaceone/api/auth0/v2/callback"
	})
	if err != nil {
		return "", err
	}

	code := callbackURL.Query().Get("code")
	if code == "" {
		return "", fmt.Errorf("auth0 callback did not include code: %s", callbackURL.String())
	}

	values := url.Values{}
	values.Set("client_id", auth0ClientID)
	values.Set("code_verifier", verifier)
	values.Set("grant_type", "authorization_code")
	values.Set("code", code)
	values.Set("redirect_uri", auth0RedirectURI)

	token, err := postToken(ctx, client, values)
	if err != nil {
		return "", err
	}

	a.storeToken(token)
	return a.accessToken, nil
}

func (a *WeWorkAuthenticator) storeToken(token authTokenResponse) {
	a.accessToken = token.AccessToken
	if token.RefreshToken != "" {
		a.refreshToken = token.RefreshToken
	}

	expiresIn := token.ExpiresIn
	if expiresIn == 0 {
		expiresIn = 3600
	}
	a.expiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
}

func authorizeURL(state, nonce, challenge, email string) (string, error) {
	values := url.Values{}
	values.Set("client_id", auth0ClientID)
	values.Set("scope", auth0Scope)
	values.Set("display", "")
	values.Set("prompt", "")
	values.Set("screen_hint", "login")
	values.Set("audience", auth0Audience)
	values.Set("redirect_uri", auth0RedirectURI)
	values.Set("ui_locales", "en-US")
	values.Set("ext-weblogin", "true")
	values.Set("login_hint", email)
	values.Set("response_type", "code")
	values.Set("response_mode", "query")
	values.Set("state", state)
	values.Set("nonce", nonce)
	values.Set("code_challenge", challenge)
	values.Set("code_challenge_method", "S256")
	values.Set("auth0Client", auth0ClientInfo)

	return "https://idp.wework.com/authorize?" + values.Encode(), nil
}

func postToken(ctx context.Context, client *http.Client, values url.Values) (authTokenResponse, error) {
	req, err := newFormRequest(ctx, http.MethodPost, "https://idp.wework.com/oauth/token", values)
	if err != nil {
		return authTokenResponse{}, err
	}
	req.Header.Set("auth0-client", auth0ClientInfo)
	req.Header.Set("origin", "https://members.wework.com")
	req.Header.Set("referer", "https://members.wework.com/")

	resp, err := client.Do(req)
	if err != nil {
		return authTokenResponse{}, err
	}
	defer resp.Body.Close()

	var token authTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return authTokenResponse{}, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 || token.AccessToken == "" {
		if token.Error != "" {
			return authTokenResponse{}, fmt.Errorf("auth0 token error: %s: %s", token.Error, token.Description)
		}

		return authTokenResponse{}, fmt.Errorf("auth0 token error: %s", resp.Status)
	}

	return token, nil
}

func getFollow(ctx context.Context, client *http.Client, rawURL string, stop func(*url.URL) bool) (*url.URL, string, error) {
	return doFollow(ctx, client, http.MethodGet, rawURL, nil, stop)
}

func postFormFollow(ctx context.Context, client *http.Client, rawURL string, values url.Values, stop func(*url.URL) bool) (*url.URL, string, error) {
	return doFollow(ctx, client, http.MethodPost, rawURL, values, stop)
}

func doFollow(ctx context.Context, client *http.Client, method, rawURL string, values url.Values, stop func(*url.URL) bool) (*url.URL, string, error) {
	for range 20 {
		req, err := newRequest(ctx, method, rawURL, values)
		if err != nil {
			return nil, "", err
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, "", err
		}

		current := resp.Request.URL
		if stop(current) {
			bodyBytes, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				return nil, "", readErr
			}

			body := string(bodyBytes)
			return current, body, nil
		}

		if resp.StatusCode < 300 || resp.StatusCode >= 400 {
			bodyBytes, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				return nil, "", readErr
			}

			body := string(bodyBytes)
			return current, body, fmt.Errorf("unexpected auth response at %s: %s", current.String(), resp.Status)
		}

		location := resp.Header.Get("Location")
		if location == "" {
			resp.Body.Close()
			return current, "", fmt.Errorf("auth redirect missing location at %s", current.String())
		}

		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		next, err := current.Parse(location)
		if err != nil {
			return current, "", err
		}

		rawURL = next.String()
		method = http.MethodGet
		values = nil
	}

	return nil, "", errors.New("too many auth redirects")
}

func newRequest(ctx context.Context, method, rawURL string, values url.Values) (*http.Request, error) {
	if method == http.MethodPost {
		return newFormRequest(ctx, method, rawURL, values)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	setBrowserHeaders(req)
	return req, nil
}

func newFormRequest(ctx context.Context, method, rawURL string, values url.Values) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	setBrowserHeaders(req)
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	return req, nil
}

func setBrowserHeaders(req *http.Request) {
	req.Header.Set("user-agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36")
	req.Header.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("accept-language", "en-US,en;q=0.9")
}

func newPKCEPair() (string, string, error) {
	verifier, err := randomURLString(32)
	if err != nil {
		return "", "", err
	}

	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomURLString(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func extractInputValue(body, name string) string {
	needle := `name="` + name + `"`
	idx := strings.Index(body, needle)
	if idx == -1 {
		needle = `name='` + name + `'`
		idx = strings.Index(body, needle)
	}
	if idx == -1 {
		return ""
	}

	valueIdx := strings.Index(body[idx:], "value=")
	if valueIdx == -1 {
		return ""
	}
	valueStart := idx + valueIdx + len("value=")
	if valueStart >= len(body) {
		return ""
	}

	quote := body[valueStart]
	if quote != '\'' && quote != '"' {
		return ""
	}
	valueStart++

	valueEnd := strings.IndexByte(body[valueStart:], quote)
	if valueEnd == -1 {
		return ""
	}

	return body[valueStart : valueStart+valueEnd]
}
