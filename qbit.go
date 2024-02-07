// Package qbit provides a few methods to interact with a qbittorrent installation.
// This package is in no way complete, and was written for a specific purpose.
// If you need more features, please open a PR or GitHub Issue with the request.
package qbit

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
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
	URL      string       `json:"url"       toml:"url"       xml:"url"       yaml:"url"`
	User     string       `json:"user"      toml:"user"      xml:"user"      yaml:"user"`
	Pass     string       `json:"pass"      toml:"pass"      xml:"pass"      yaml:"pass"`
	HTTPPass string       `json:"http_pass" toml:"http_pass" xml:"http_pass" yaml:"http_pass"`
	HTTPUser string       `json:"http_user" toml:"http_user" xml:"http_user" yaml:"http_user"`
	Client   *http.Client `json:"-"         toml:"-"         xml:"-"         yaml:"-"`
}

// Qbit is what you get in return for passing in a valid Config to New().
type Qbit struct {
	config *Config
	auth   string
	client *http.Client
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

// Category represents a torrent category in Qbit.
type Category struct {
	Name     string `json:"name"`
	SavePath string `json:"savePath"`
}

func NewNoAuth(config *Config) (*Qbit, error) {
	return newConfig(context.TODO(), config, false)
}

func New(ctx context.Context, config *Config) (*Qbit, error) {
	return newConfig(ctx, config, true)
}

func newConfig(ctx context.Context, config *Config, login bool) (*Qbit, error) {
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
		auth:   auth,
		client: httpClient,
	}

	if !login {
		return qbit, nil
	}

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	return qbit, qbit.login(ctx)
}

// login is called once from New().
func (q *Qbit) login(ctx context.Context) error {
	params := make(url.Values)
	params.Add("username", q.config.User)
	params.Add("password", q.config.Pass)
	post := strings.NewReader(params.Encode())

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

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Ok.") {
		return fmt.Errorf("%w: %s: %s: %s", ErrLoginFailed, resp.Status, req.URL, string(body))
	}

	return nil
}

// SetTorrentCategory updates the category for 1 or more torrents.
func (q *Qbit) SetTorrentCategory(category string, torrentHashes ...string) error {
	return q.SetTorrentCategoryContext(context.Background(), category, torrentHashes...)
}

// SetTorrentCategoryContext updates the category for 1 or more torrents.
func (q *Qbit) SetTorrentCategoryContext(ctx context.Context, category string, torrentHashes ...string) error {
	values := url.Values{}
	values.Set("category", category)
	values.Set("hashes", strings.Join(torrentHashes, "|"))

	var into map[string]interface{}
	if err := q.postReq(ctx, "api/v2/torrents/setCategory", values, into); err != nil {
		return err
	}

	return nil
}

// GetCategories returns all the categories in Qbit.
func (q *Qbit) GetCategories() (map[string]*Category, error) {
	return q.GetCategoriesContext(context.Background())
}

// GetCategoriesContext returns all the categories in Qbit.
func (q *Qbit) GetCategoriesContext(ctx context.Context) (map[string]*Category, error) {
	cats := map[string]*Category{}
	if err := q.getReq(ctx, "api/v2/torrents/categories", &cats); err != nil {
		return nil, err
	}

	return cats, nil
}

// GetXfers returns data about all transfers/downloads in the Qbit client.
func (q *Qbit) GetXfers() ([]*Xfer, error) {
	return q.GetXfersContext(context.Background())
}

// GetXfersContext returns data about all transfers/downloads in the Qbit client.
func (q *Qbit) GetXfersContext(ctx context.Context) ([]*Xfer, error) {
	xfers := []*Xfer{}
	if err := q.getReq(ctx, "api/v2/torrents/info", &xfers); err != nil {
		return nil, err
	}

	return xfers, nil
}

func (q *Qbit) getReq(ctx context.Context, path string, into interface{}) error {
	return q.req(ctx, http.MethodGet, q.config.URL+path, nil, into, true)
}

func (q *Qbit) postReq(ctx context.Context, path string, values url.Values, into interface{}) error {
	return q.req(ctx, http.MethodPost, q.config.URL+path, values, into, true)
}

func (q *Qbit) req(ctx context.Context, method, uri string, val url.Values, into interface{}, loop bool) error {
	var body io.Reader

	if method == http.MethodPost {
		body = bytes.NewBufferString(val.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, method, uri, body)
	if err != nil {
		return fmt.Errorf("creating '%s' request: %w", method, err)
	}

	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		val.Set("filter", "all")
		req.URL.RawQuery = val.Encode()
	}

	req.Header.Set("Accept", "application/json")

	if q.auth != "" {
		req.Header.Set("Authorization", q.auth)
	}

	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("%s failed: %w", method, err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		if err := q.login(ctx); err != nil {
			return err
		}

		if loop { // try again after logging in.
			return q.req(ctx, method, uri, val, into, false)
		}

		return fmt.Errorf("%s: %w", resp.Status, err)
	}

	return nil
}
