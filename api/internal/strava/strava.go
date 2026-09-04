// Package strava talks to the Strava v3 API: the OAuth exchange, token
// refresh, and listing an athlete's recent activities.
//
// It is deliberately thin. Only the fields the app actually uses are decoded,
// and nothing here touches the database — the caller owns storage, so this
// package stays testable against an httptest server.
package strava

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// AuthURL is where the browser is sent to authorise.
	AuthURL = "https://www.strava.com/oauth/authorize"
	// TokenURL exchanges a code, and refreshes an expired token.
	TokenURL = "https://www.strava.com/oauth/token"
	// APIBase is the v3 root.
	APIBase = "https://www.strava.com/api/v3"

	// Scope is the minimum needed to read activities, including those the
	// athlete has marked private — a private morning walk still counts.
	Scope = "read,activity:read_all"
)

// Client calls Strava on behalf of one application.
type Client struct {
	ClientID     string
	ClientSecret string
	// BaseURL overrides APIBase, and TokenEndpoint overrides TokenURL. Both
	// exist so tests can point at an httptest server.
	BaseURL       string
	TokenEndpoint string
	HTTP          *http.Client
}

// New builds a client with a sane timeout.
func New(clientID, clientSecret string) *Client {
	return &Client{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		HTTP:         &http.Client{Timeout: 30 * time.Second},
	}
}

// Configured reports whether credentials are present. Without them the whole
// feature is simply off, which is a valid state rather than a startup failure.
func (c *Client) Configured() bool {
	return c != nil && strings.TrimSpace(c.ClientID) != "" && strings.TrimSpace(c.ClientSecret) != ""
}

func (c *Client) api() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return APIBase
}

func (c *Client) tokenURL() string {
	if c.TokenEndpoint != "" {
		return c.TokenEndpoint
	}
	return TokenURL
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// AuthorizeURL builds the consent URL.
//
// approval_prompt=auto so a returning athlete is not asked again, and state
// carries the CSRF token the callback checks.
func (c *Client) AuthorizeURL(redirectURI, state string) string {
	q := url.Values{}
	q.Set("client_id", c.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("approval_prompt", "auto")
	q.Set("scope", Scope)
	q.Set("state", state)
	return AuthURL + "?" + q.Encode()
}

// Token is a Strava access token and the athlete it belongs to.
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	Scope        string `json:"scope"`
	Athlete      struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	} `json:"athlete"`
}

// Expired reports whether the token needs refreshing.
//
// A minute of slack, so a token that expires mid-request is refreshed first
// rather than failing the call.
func (t Token) Expired() bool {
	return t.ExpiresAt == 0 || time.Now().Add(time.Minute).Unix() >= t.ExpiresAt
}

// Exchange trades an authorisation code for tokens.
func (c *Client) Exchange(ctx context.Context, code string) (*Token, error) {
	return c.token(ctx, url.Values{
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
	})
}

// Refresh renews an expired access token.
//
// Strava rotates the refresh token on some responses, so the caller must store
// whatever comes back rather than keeping the one it sent.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (*Token, error) {
	return c.token(ctx, url.Values{
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	})
}

func (c *Client) token(ctx context.Context, form url.Values) (*Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL(),
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("strava: token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("strava: reading token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("strava: token request failed (%d): %s", resp.StatusCode, snippet(body))
	}

	var tok Token
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("strava: decoding token: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("strava: token response contained no access token")
	}
	return &tok, nil
}

// Activity is the subset of a Strava activity the app stores.
type Activity struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	SportType string `json:"sport_type"`
	// Trainer marks a treadmill, turbo or other stationary session.
	Trainer bool `json:"trainer"`
	Commute bool `json:"commute"`
	// Manual activities were typed in rather than recorded; they still count.
	Manual         bool     `json:"manual"`
	MovingTime     int      `json:"moving_time"`
	ElapsedTime    int      `json:"elapsed_time"`
	Distance       float64  `json:"distance"`
	TotalElevation float64  `json:"total_elevation_gain"`
	Calories       *float64 `json:"calories"`
	AverageHR      *float64 `json:"average_heartrate"`
	MaxHR          *float64 `json:"max_heartrate"`
	AverageSpeed   float64  `json:"average_speed"`
	// StartDateLocal is the athlete's wall-clock start. It carries a Z suffix
	// that is a lie — Strava shifts the time into local and leaves the offset
	// off — so it is parsed as a naive local timestamp, never as UTC.
	StartDateLocal string `json:"start_date_local"`
	StartDate      string `json:"start_date"`
}

// LocalDate returns the calendar date the activity happened on, in the
// athlete's own timezone.
//
// This is what makes an evening walk land on the day it was walked. Using the
// UTC start would push anything after 19:00 Eastern onto tomorrow.
func (a Activity) LocalDate() string {
	if t, err := time.Parse("2006-01-02T15:04:05Z", a.StartDateLocal); err == nil {
		return t.Format("2006-01-02")
	}
	if len(a.StartDateLocal) >= 10 {
		return a.StartDateLocal[:10]
	}
	if len(a.StartDate) >= 10 {
		return a.StartDate[:10]
	}
	return ""
}

// StartTime returns the true instant the activity began.
func (a Activity) StartTime() time.Time {
	if t, err := time.Parse(time.RFC3339, a.StartDate); err == nil {
		return t
	}
	return time.Time{}
}

// indoorSports never happen outside, whatever the athlete's device reports.
var indoorSports = map[string]bool{
	"VirtualRide":                   true,
	"VirtualRun":                    true,
	"VirtualRow":                    true,
	"WeightTraining":                true,
	"Workout":                       true,
	"Yoga":                          true,
	"Crossfit":                      true,
	"Elliptical":                    true,
	"StairStepper":                  true,
	"Pilates":                       true,
	"HighIntensityIntervalTraining": true,
}

// outdoorSports only happen outside, so they override a mis-set trainer flag.
var outdoorSports = map[string]bool{
	"Hike":           true,
	"TrailRun":       true,
	"AlpineSki":      true,
	"BackcountrySki": true,
	"NordicSki":      true,
	"Snowboard":      true,
	"Snowshoe":       true,
	"Kayaking":       true,
	"Canoeing":       true,
	"Surfing":        true,
	"RockClimbing":   true,
	"Sail":           true,
	"Kitesurf":       true,
	"Windsurf":       true,
}

// restIntervalSports are the sports whose rest is part of the workout.
//
// Strava's moving time stops whenever you do, which is the right measure for a
// run or a ride and the wrong one for a session built out of intervals: a
// forty-five minute swim reports thirty-two, because the seconds spent resting
// at the wall are not swimming, and the same applies to the rest between sets
// in a lifting session. For these, the time the session actually took is what
// a "45-minute workout" means.
var restIntervalSports = map[string]bool{
	"Swim":                          true,
	"WeightTraining":                true,
	"Crossfit":                      true,
	"HighIntensityIntervalTraining": true,
	"RockClimbing":                  true,
	"Bouldering":                    true,
	"Workout":                       true,
	"Yoga":                          true,
	"Pilates":                       true,
}

// elapsedSanityFactor bounds how far elapsed time may exceed moving time
// before it is treated as a watch left running rather than as rest.
//
// Three is comfortably above a hard interval session and well below the hour
// that a forgotten stop adds.
const elapsedSanityFactor = 3

// SessionMinutes is how long an activity counts for.
//
// Moving time for anything that moves, elapsed time for the sports whose rest
// is part of the work — and moving time again when the elapsed figure is so
// far past it that the watch was plainly left running.
func SessionMinutes(sportType, actType string, movingSeconds, elapsedSeconds int) int {
	seconds := movingSeconds
	if restIntervalSports[sportType] || restIntervalSports[actType] {
		if elapsedSeconds > seconds && elapsedSeconds <= seconds*elapsedSanityFactor {
			seconds = elapsedSeconds
		}
	}
	return seconds / 60
}

// Classify decides whether an activity counts as the indoor or the outdoor
// session.
//
// Strava has no "was this outside" field, so this reads the three signals it
// does give, in order of how much they can be trusted:
//
//  1. An unambiguously outdoor sport — a hike is a hike, even if the trainer
//     flag got set by a mis-configured device.
//  2. The trainer flag, which the athlete or their device sets for a treadmill
//     or turbo session.
//  3. The sport type, for the gym activities that are never outdoors.
//
// Anything left over (a plain Run, Ride or Walk with no trainer flag) is
// treated as outdoor, because that is what it almost always is — and the
// classification is editable afterwards, so the cheap error is the recoverable
// one.
func Classify(a Activity) string {
	if outdoorSports[a.SportType] || outdoorSports[a.Type] {
		return "outdoor"
	}
	if a.Trainer {
		return "indoor"
	}
	if indoorSports[a.SportType] || indoorSports[a.Type] {
		return "indoor"
	}
	return "outdoor"
}

// Activities lists the athlete's activities after a given time, newest first.
func (c *Client) Activities(ctx context.Context, accessToken string, after time.Time, perPage int) ([]Activity, error) {
	if perPage <= 0 || perPage > 200 {
		perPage = 60
	}

	q := url.Values{}
	q.Set("per_page", strconv.Itoa(perPage))
	if !after.IsZero() {
		q.Set("after", strconv.FormatInt(after.Unix(), 10))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.api()+"/athlete/activities?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("strava: listing activities: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("strava: reading activities: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, ErrUnauthorized
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, ErrRateLimited
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("strava: activities failed (%d): %s", resp.StatusCode, snippet(body))
	}

	var out []Activity
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("strava: decoding activities: %w", err)
	}
	return out, nil
}

// Sentinel errors the caller reacts to differently: a revoked authorisation
// needs the athlete to reconnect, a rate limit just needs waiting out.
var (
	ErrUnauthorized = fmt.Errorf("strava: authorisation was rejected; reconnect the account")
	ErrRateLimited  = fmt.Errorf("strava: rate limited; try again shortly")
)

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
