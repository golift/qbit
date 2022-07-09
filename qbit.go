// Package qbit provides a few methods to interact with a qbittorrent installation.
// This package is in no way complete, and was written for a specific purpose.
// If you need more features, please open a PR or GitHub Issue with the request.
package qbit

import (
	"context"
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

// Package defaults.
const (
	DefaultTimeout = 1 * time.Minute
)

// Custom errors returned by this package.
var (
	ErrLoginFailed = fmt.Errorf("authentication failed")
)

// Config is the input data needed to return a Qbit struct.
// This is setup to allow you to easily pass this data in from a config file.
type Config struct {
	URL      string       `json:"url" toml:"url" xml:"url" yaml:"url"`
	User     string       `json:"user" toml:"user" xml:"user" yaml:"user"`
	Pass     string       `json:"pass" toml:"pass" xml:"pass" yaml:"pass"`
	HTTPPass string       `json:"http_pass" toml:"http_pass" xml:"http_pass" yaml:"http_pass"`
	HTTPUser string       `json:"http_user" toml:"http_user" xml:"http_user" yaml:"http_user"`
	Client   *http.Client `json:"-" toml:"-" xml:"-" yaml:"-"`
}

// Qbit is what you get in return for passing in a valid Config to New().
type Qbit struct {
	config *Config
	client *client
}

type client struct {
	auth   string
	cookie bool
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

func NewNoAuth(config *Config) (*Qbit, error) {
	return newConfig(config, false)
}

func New(config *Config) (*Qbit, error) {
	return newConfig(config, true)
}

func newConfig(config *Config, login bool) (*Qbit, error) {
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

	httpClient := config.Client
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	httpClient.Jar = jar

	qbit := &Qbit{
		config: config,
		client: &client{
			auth:   auth,
			Client: httpClient,
		},
	}

	if !login {
		return qbit, nil
	}

	return qbit, qbit.login()
}

// login is called once from New().
func (q *Qbit) login() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	post := strings.NewReader("username=" + q.config.User + "&password=" + q.config.Pass)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.config.URL+"api/v2/auth/login", post)
	if err != nil {
		return fmt.Errorf("creating login request: %w", err)
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Ok.") {
		return fmt.Errorf("%w: %s: %s: %s", ErrLoginFailed, resp.Status, req.URL, string(body))
	}

	q.client.cookie = true

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
	return q.GetXfersContext(context.Background())
}

// GetXfersContext returns data about all transfers/downloads in the Qbit client.
func (q *Qbit) GetXfersContext(ctx context.Context) ([]*Xfer, error) {
	if !q.client.cookie {
		if err := q.login(); err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, q.config.URL+"api/v2/torrents/info", nil)
	if err != nil {
		return nil, fmt.Errorf("creating info request: %w", err)
	}

	req.URL.RawQuery = "filter=all"

	resp, err := q.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("info req failed: %w", err)
	}
	defer resp.Body.Close()

	xfers := []*Xfer{}
	if err := json.NewDecoder(resp.Body).Decode(&xfers); err != nil {
		q.client.cookie = false
		return nil, fmt.Errorf("decoding body failed: %w", err)
	}

	return xfers, nil
}
