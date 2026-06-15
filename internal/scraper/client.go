package scraper

import (
	"fmt"
	"net/http"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

var (
	BotIDSupplier func() string
	Version       = "dev"
	CommitSHA     = "unknown"
	RepoURL       = "github.com/geckyzz/contourgo"
)

func init() {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				if CommitSHA == "unknown" || CommitSHA == "" {
					CommitSHA = setting.Value
					if len(CommitSHA) > 7 {
						CommitSHA = CommitSHA[:7]
					}
				}
			case "vcs.url":
				url := setting.Value
				url = strings.TrimSuffix(url, ".git")
				url = strings.TrimPrefix(url, "https://")
				url = strings.TrimPrefix(url, "git@")
				url = strings.Replace(url, ":", "/", 1)
				parts := strings.Split(url, "/")
				if len(parts) >= 3 && parts[0] == "github.com" {
					RepoURL = fmt.Sprintf("github.com/%s/%s", parts[1], parts[2])
				}
			}
		}
	}
}

type UserAgentTransport struct {
	Transport http.RoundTripper
}

func (t *UserAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())

	botID := "unknown"
	if BotIDSupplier != nil {
		if id := BotIDSupplier(); id != "" {
			botID = id
		}
	}

	ua := fmt.Sprintf(
		"Go-http-client/1.1 (%s; en-US) Contourgo/%s+%s (+https://discord.com/users/%s; +https://%s)",
		runtime.GOOS,
		Version,
		CommitSHA,
		botID,
		RepoURL,
	)
	req.Header.Set("User-Agent", ua)

	if t.Transport != nil {
		return t.Transport.RoundTrip(req)
	}
	return http.DefaultTransport.RoundTrip(req)
}

func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: &UserAgentTransport{},
	}
}
