// Package qbit provides a few methods to interact with a qbittorrent installation.
// This package is in no way complete, and was written for a specific purpose.
// If you need more features, please open a PR or GitHub Issue with the request.
package qbit

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"
)

// Custom errors returned by this package.
var (
	ErrLoginFailed = fmt.Errorf("authentication failed")
)

// Logger is an optional generic interface to allow this library to log debug messages.
type Logger func(msg string, fmt ...interface{})

// Config is the input data needed to return a Qbit struct.
// This is setup to allow you to easily pass this data in from a config file.
type Config struct {
	URL       string   `json:"url" toml:"url" xml:"url" yaml:"url"`
	User      string   `json:"user" toml:"user" xml:"user" yaml:"user"`
	Pass      string   `json:"pass" toml:"pass" xml:"pass" yaml:"pass"`
	HTTPPass  string   `json:"http_pass" toml:"http_pass" xml:"http_pass" yaml:"http_pass"`
	HTTPUser  string   `json:"http_user" toml:"http_user" xml:"http_user" yaml:"http_user"`
	Timeout   Duration `json:"timeout" toml:"timeout" xml:"timeout" yaml:"timeout"`
	VerifySSL bool     `json:"verify_ssl" toml:"verify_ssl" xml:"verify_ssl" yaml:"verify_ssl"`
	DebugLog  Logger   `json:"-" toml:"-" xml:"-" yaml:"-"`
}

// Qbit is what you get in return for passing in a valid Config to New().
type Qbit struct {
	config *Config
	*client
}

// Duration is used to parse durations from a config file.
type Duration struct{ time.Duration }

type client struct {
	auth string
	*http.Client
}

// Xfer is a transfer from the torrents/info endpoint.
type Xfer struct {
	AddedOn           int     `json:"added_on"`
	AmountLeft        int     `json:"amount_left"`
	AutoTmm           bool    `json:"auto_tmm"`
	Availability      float64 `json:"availability"`
	Category          string  `json:"category"`
	Completed         int     `json:"completed"`
	CompletionOn      int     `json:"completion_on"`
	ContentPath       string  `json:"content_path"`
	DlLimit           int     `json:"dl_limit"`
	Dlspeed           int     `json:"dlspeed"`
	Downloaded        int     `json:"downloaded"`
	DownloadedSession int     `json:"downloaded_session"`
	Eta               int     `json:"eta"`
	FLPiecePrio       bool    `json:"f_l_piece_prio"`
	ForceStart        bool    `json:"force_start"`
	Hash              string  `json:"hash"`
	LastActivity      int     `json:"last_activity"`
	MagnetURI         string  `json:"magnet_uri"`
	MaxRatio          float64 `json:"max_ratio"`
	MaxSeedingTime    int     `json:"max_seeding_time"`
	Name              string  `json:"name"`
	NumComplete       int     `json:"num_complete"`
	NumIncomplete     int     `json:"num_incomplete"`
	NumLeechs         int     `json:"num_leechs"`
	NumSeeds          int     `json:"num_seeds"`
	Priority          int     `json:"priority"`
	Progress          float64 `json:"progress"`
	Ratio             float64 `json:"ratio"`
	RatioLimit        float64 `json:"ratio_limit"`
	SavePath          string  `json:"save_path"`
	SeedingTime       int64   `json:"seeding_time"`
	SeedingTimeLimit  int64   `json:"seeding_time_limit"`
	SeenComplete      int64   `json:"seen_complete"`
	SeqDl             bool    `json:"seq_dl"`
	Size              int64   `json:"size"`
	State             string  `json:"state"`
	SuperSeeding      bool    `json:"super_seeding"`
	Tags              string  `json:"tags"`
	TimeActive        int64   `json:"time_active"`
	TotalSize         int64   `json:"total_size"`
	Tracker           string  `json:"tracker"`
	TrackersCount     int     `json:"trackers_count"`
	UpLimit           int64   `json:"up_limit"`
	Uploaded          int64   `json:"uploaded"`
	UploadedSession   int64   `json:"uploaded_session"`
	Upspeed           int64   `json:"upspeed"`
}

// UnmarshalText parses a duration type from a config file.
func (d *Duration) UnmarshalText(data []byte) (err error) {
	d.Duration, err = time.ParseDuration(string(data))
	return
}

func New(config *Config) (*Qbit, error) {
	// The cookie jar is used to auth Qbit.
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, fmt.Errorf("cookiejar.New(publicsuffix): %w", err)
	}

	config.URL = strings.TrimSuffix(config.URL, "/") + "/"

	// This app allows http auth, in addition to qbit web username/password.
	auth := config.HTTPUser + ":" + config.HTTPPass
	if auth != ":" {
		auth = "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
	} else {
		auth = ""
	}

	if config.Timeout.Duration == 0 {
		config.Timeout.Duration = 1 * time.Minute
	}

	qbit := &Qbit{
		config: config,
		client: &client{
			auth: auth,
			Client: &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: config.VerifySSL}, //nolint:gosec
				},
				Jar:     jar,
				Timeout: config.Timeout.Duration,
			},
		},
	}

	return qbit, qbit.login()
}

// login is called once from New().
func (q *Qbit) login() error {
	ctx, cancel := context.WithTimeout(context.Background(), q.config.Timeout.Duration)
	defer cancel()

	post := strings.NewReader("username=" + q.config.User + "&password=" + q.config.Pass)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.config.URL+"api/v2/auth/login", post)
	if err != nil {
		return fmt.Errorf("creating login request: %w", err)
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := q.Do(req)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Ok.") {
		return fmt.Errorf("%w: %s: %s: %s", ErrLoginFailed, resp.Status, req.URL, string(body))
	}

	return nil
}

// Do allows overriding the http request parameters in aggregate.
func (c *client) Do(req *http.Request) (*http.Response, error) {
	if c.auth != "" {
		req.Header.Set("Authorization", c.auth)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return resp, fmt.Errorf("making request: %w", err)
	}

	return resp, nil
}

// GetXfers returns data about all transfers/downloads in the Qbit client.
func (q *Qbit) GetXfers() ([]*Xfer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), q.config.Timeout.Duration)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, q.config.URL+"api/v2/torrents/info", nil)
	if err != nil {
		return nil, fmt.Errorf("creating info request: %w", err)
	}

	req.URL.RawQuery = "filter=all"

	resp, err := q.Do(req)
	if err != nil {
		return nil, fmt.Errorf("info req failed: %w", err)
	}
	defer resp.Body.Close()

	xfers := []*Xfer{}

	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&xfers); err != nil {
		return nil, fmt.Errorf("decoding body failed: %w", err)
	}

	return xfers, nil
}
