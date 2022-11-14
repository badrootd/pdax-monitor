package auth

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"strings"

	"github.com/go-kit/log"
)

// PDAXAuthService is used to get authToken by passing auth procedure through user sign-in.
type PDAXAuthService struct {
	Username       string
	Password       string
	AuthURL        string
	AuthRefreshURL string
	captchaSolver  CaptchaSolver
	Logger         log.Logger
}

// NewAuthService instantiates PDAXAuthService.
func NewAuthService(authURL string, authRefreshURL string, options ...ConfigOption) PDAXAuthService {
	auth := PDAXAuthService{
		AuthURL:        authURL,
		AuthRefreshURL: authRefreshURL,
	}

	for _, opt := range options {
		opt(&auth)
	}

	return auth
}

// ConfigOption configures the PDAXAuthService.
type ConfigOption func(*PDAXAuthService)

// WithCredentials struct contains credentials for sign-in.
func WithCredentials(username, password string) ConfigOption {
	return func(auth *PDAXAuthService) {
		auth.Username = username
		auth.Password = password
	}
}

// WithCaptchaSolver configures captcha solver for the auth service.
func WithCaptchaSolver(cs CaptchaSolver) ConfigOption {
	return func(m *PDAXAuthService) {
		m.captchaSolver = cs
	}
}

// WithLogger configures a logger to debug the service.
func WithLogger(l log.Logger) ConfigOption {
	return func(m *PDAXAuthService) {
		m.Logger = l
	}
}

type device struct {
	H  string `json:"h"`
	L  string `json:"l"`
	R  string `json:"r"`
	H2 string `json:"h2"`
}

type pdaxLoginForm struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Trusted  bool   `json:"trusted"`
	Captcha  string `json:"captcha"`
	Platform string `json:"platform"`
	Device   device `json:"device"`
}

// Login is used to authenticate in PDAX.
func (a *PDAXAuthService) Login() (string, error) {
	gcaptcha, err := a.captchaSolver.Solve()
	if err != nil {
		return "", fmt.Errorf("2captcha.com solution error: %v", err)
	}

	// error is always nil
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar, // cookies are used to have access for getting refresh token
	}

	form := pdaxLoginForm{
		a.Username,
		a.Password,
		false,
		gcaptcha,
		"null",
		device{
			"94129614769af80e6b7cd7fc68294349",
			"ru-RU",
			"1920x1080",
			"6ed8863b119af2f53411e620bb1ebd8f35ce1da9e0d7a04b26bbdd1f11d5c156",
		},
	}
	formJSON, err := json.Marshal(form)
	if err != nil {
		return "", fmt.Errorf("login form marshall error: %v", err)
	}

	err = a.getJWTToken(client, formJSON)
	if err != nil {
		return "", err
	}

	return a.getRefreshedToken(client, formJSON)
}

func (a *PDAXAuthService) getJWTToken(c *http.Client, formJSON []byte) error {
	req, err := http.NewRequest("POST", a.AuthURL, strings.NewReader(string(formJSON)))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func (a *PDAXAuthService) getRefreshedToken(client *http.Client, formJSON []byte) (string, error) {
	var err error
	req, err := http.NewRequest("GET", a.AuthRefreshURL, strings.NewReader(string(formJSON)))
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	type AuthToken struct {
		AuthToken string `json:"authToken"`
	}
	var refreshToken AuthToken
	err = json.Unmarshal(data, &refreshToken)
	if err != nil {
		return "", err
	}

	return refreshToken.AuthToken, nil
}
