package scraper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type NyaaScraper struct {
	proxyURL string
	client   *http.Client
}

func NewNyaaScraper(proxyURL string) *NyaaScraper {
	return &NyaaScraper{
		proxyURL: proxyURL,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (s *NyaaScraper) FetchTorrents(username string, keyword string, page int, sort string, order string) ([]NyaaTorrent, int, error) {
	var endpoint string
	if username != "" {
		endpoint = fmt.Sprintf("%s/nyaa/v1/user/%s/uploads?p=%d&s=%s&o=%s", s.proxyURL, url.PathEscape(username), page, url.QueryEscape(sort), url.QueryEscape(order))
	} else {
		endpoint = fmt.Sprintf("%s/nyaa/v1/?q=%s&p=%d&s=%s&o=%s", s.proxyURL, url.QueryEscape(keyword), page, url.QueryEscape(sort), url.QueryEscape(order))
	}

	resp, err := s.client.Get(endpoint)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("HTTP error %d", resp.StatusCode)
	}

	var result NyaaSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, err
	}

	return result.Torrents, result.Pagination.TotalPages, nil
}

func (s *NyaaScraper) FetchComments(torrentID string) ([]NyaaComment, error) {
	endpoint := fmt.Sprintf("%s/nyaa/v1/view/%s/comments", s.proxyURL, torrentID)
	resp, err := s.client.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error %d", resp.StatusCode)
	}

	var comments []NyaaComment
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return nil, err
	}

	return comments, nil
}
