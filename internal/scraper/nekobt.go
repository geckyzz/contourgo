package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type NekoBTScraper struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewNekoBTScraper(apiKey string) *NekoBTScraper {
	return &NekoBTScraper{
		baseURL: "https://nekobt.to/api/v1",
		apiKey:  apiKey,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *NekoBTScraper) doRequest(req *http.Request) ([]byte, error) {
	if s.apiKey != "" {
		req.AddCookie(&http.Cookie{Name: "ssid", Value: s.apiKey})
	}
	req.Header.Set("User-Agent", "ContourGo/1.0")

	for {
		resp, err := s.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode == 429 {
			retryAfter := 5.0
			// Try to parse JSON retry_after
			body, _ := io.ReadAll(resp.Body)
			var nResp struct {
				RetryAfter float64 `json:"retry_after"`
			}
			if err := json.Unmarshal(body, &nResp); err == nil && nResp.RetryAfter > 0 {
				retryAfter = nResp.RetryAfter
			} else {
				// Fallback to header
				if ra := resp.Header.Get("Retry-After"); ra != "" {
					if val, err := strconv.ParseFloat(ra, 64); err == nil {
						retryAfter = val
					}
				}
			}
			log.Printf("[nekoBT] Rate limited. Retrying after %.2fs...", retryAfter)
			time.Sleep(time.Duration(retryAfter*1000) * time.Millisecond)

			// Re-create request for retry since body might be closed or state changed
			newReq, _ := http.NewRequest(req.Method, req.URL.String(), nil)
			for k, v := range req.Header {
				newReq.Header[k] = v
			}
			if s.apiKey != "" {
				newReq.AddCookie(&http.Cookie{Name: "ssid", Value: s.apiKey})
			}
			req = newReq
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP error %d", resp.StatusCode)
		}

		return io.ReadAll(resp.Body)
	}
}

func (s *NekoBTScraper) SearchTorrents(params url.Values) ([]NekoBTTorrent, error) {
	u := fmt.Sprintf("%s/torrents/search?%s", s.baseURL, params.Encode())
	req, _ := http.NewRequest("GET", u, nil)

	body, err := s.doRequest(req)
	if err != nil {
		return nil, err
	}

	var nResp NekoBTResponse
	if err := json.Unmarshal(body, &nResp); err != nil {
		return nil, err
	}

	if nResp.Error {
		return nil, fmt.Errorf("nekoBT error: %s", nResp.Message)
	}

	// Data is interface{}, need to re-marshal or use map
	dataBytes, _ := json.Marshal(nResp.Data)
	var searchResult NekoBTSearchResult
	if err := json.Unmarshal(dataBytes, &searchResult); err != nil {
		return nil, err
	}

	return searchResult.Results, nil
}

func (s *NekoBTScraper) FetchComments(torrentID string, title string) ([]NekoBTComment, error) {
	// 1. Fetch Torrent Details first to get uploader/staff info
	uDetails := fmt.Sprintf("%s/torrents/%s", s.baseURL, torrentID)
	reqDetails, _ := http.NewRequest("GET", uDetails, nil)
	bodyDetails, err := s.doRequest(reqDetails)
	if err != nil {
		return nil, err
	}

	var nRespDetails struct {
		Error bool `json:"error"`
		Data  struct {
			Uploader struct {
				ID string `json:"id"`
			} `json:"uploader"`
			Groups []struct {
				Members []struct {
					ID string `json:"id"`
				} `json:"members"`
			} `json:"groups"`
		} `json:"data"`
	}
	json.Unmarshal(bodyDetails, &nRespDetails)

	uploaderID := nRespDetails.Data.Uploader.ID
	var contributorIDs []string
	for _, g := range nRespDetails.Data.Groups {
		for _, m := range g.Members {
			contributorIDs = append(contributorIDs, m.ID)
		}
	}

	// 2. Fetch Comments
	u := fmt.Sprintf("%s/torrents/%s/comments", s.baseURL, torrentID)
	req, _ := http.NewRequest("GET", u, nil)

	body, err := s.doRequest(req)
	if err != nil {
		return nil, err
	}

	var nResp NekoBTResponse
	if err := json.Unmarshal(body, &nResp); err != nil {
		return nil, err
	}

	if nResp.Error {
		return nil, fmt.Errorf("nekoBT error: %s", nResp.Message)
	}

	dataBytes, _ := json.Marshal(nResp.Data)
	var comments []NekoBTComment
	if err := json.Unmarshal(dataBytes, &comments); err != nil {
		return nil, err
	}

	// Flatten tree and inject metadata
	return s.flattenComments(comments, "", torrentID, title, uploaderID, contributorIDs), nil
}

func (s *NekoBTScraper) flattenComments(tree []NekoBTComment, parentText string, torrentID, title string, uploaderID string, contributorIDs []string) []NekoBTComment {
	var flat []NekoBTComment
	for _, c := range tree {
		c.ParentText = parentText
		c.TorrentID = torrentID
		c.Title = title
		c.UploaderID = uploaderID
		c.ContributorIDs = contributorIDs

		if c.CreatedAt == 0 {
			c.CreatedAt = DecodeNekoBTSnowflake(c.ID)
		}

		flat = append(flat, c)
		if len(c.Children) > 0 {
			flat = append(flat, s.flattenComments(c.Children, c.Text, torrentID, title, uploaderID, contributorIDs)...)
		}
	}
	return flat
}
