package scraper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type AnirenaTorrent struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	InfoHashV1   string   `json:"info_hash_v1"`
	InfoHashV2   *string  `json:"info_hash_v2"`
	SizeFmt      string   `json:"size_fmt"`
	Completed    int      `json:"completed"`
	Seeders      int      `json:"seeders"`
	Leechers     int      `json:"leechers"`
	Languages    []string `json:"languages"`
	CommentCount int      `json:"comment_count"`
	CreatedAt    string   `json:"created_at"`
	CatSlug      string   `json:"cat_slug"`
	SubSlug      string   `json:"sub_slug"`
	GroupName    *string  `json:"group_name"`
	Uploader     string   `json:"uploader"`
	Magnet       string   `json:"magnet"`
}

func (t *AnirenaTorrent) FullTitle() string {
	if t.GroupName != nil && *t.GroupName != "" {
		return fmt.Sprintf("[%s] %s", *t.GroupName, t.Title)
	}
	return t.Title
}

type AnirenaSearchResult struct {
	Total      int              `json:"total"`
	Page       int              `json:"page"`
	PerPage    int              `json:"per_page"`
	TotalPages int              `json:"total_pages"`
	Torrents   []AnirenaTorrent `json:"torrents"`
}

type AnirenaComment struct {
	ID               string  `json:"id"`
	UserID           string  `json:"user_id"`
	Username         string  `json:"username"`
	Role             string  `json:"role"`
	AuthorBanned     bool    `json:"author_banned"`
	Body             string  `json:"body"`
	CreatedAt        string  `json:"created_at"`
	EditedAt         *string `json:"edited_at"`
	EditedByUsername *string `json:"edited_by_username"`
	DeletedAt        *string `json:"deleted_at"`
}

func (c *AnirenaComment) GetTimestamp() int64 {
	parsedTime, err := time.Parse("2006-01-02 15:04:05", c.CreatedAt)
	if err == nil {
		return parsedTime.Unix()
	}
	return time.Now().Unix()
}

type AnirenaCommentsResult struct {
	TorrentID  string           `json:"torrent_id"`
	Page       int              `json:"page"`
	PerPage    int              `json:"per_page"`
	Total      int              `json:"total"`
	TotalPages int              `json:"total_pages"`
	Comments   []AnirenaComment `json:"comments"`
}

type AnirenaScraper struct {
	baseURL     string
	apiKey      string
	client      *http.Client
	token       string
	tokenExpiry time.Time
	mu          sync.Mutex
}

func NewAnirenaScraper(apiKey string) *AnirenaScraper {
	return &AnirenaScraper{
		baseURL: "https://www.anirena.com",
		apiKey:  apiKey,
		client:  NewHTTPClient(15 * time.Second),
	}
}

// GetToken returns the cached bearer token or fetches/refreshes a new one.
func (s *AnirenaScraper) GetToken() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.token != "" && time.Now().Before(s.tokenExpiry) {
		return s.token, nil
	}

	if s.apiKey != "" {
		endpoint := fmt.Sprintf("%s/api/v1/auth/token", s.baseURL)
		req, err := http.NewRequest("POST", endpoint, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", fmt.Sprintf("ApiKey %s", s.apiKey))

		resp, err := s.client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("auth/token returned HTTP status %d", resp.StatusCode)
		}

		var authResp struct {
			Token     string `json:"token"`
			ExpiresIn int    `json:"expires_in"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
			return "", err
		}

		s.token = authResp.Token
		// Buffer token expiry by 30 seconds
		expiresIn := authResp.ExpiresIn
		if expiresIn <= 0 {
			expiresIn = 3600
		}
		s.tokenExpiry = time.Now().Add(time.Duration(expiresIn)*time.Second - 30*time.Second)
		return s.token, nil
	}

	return "", fmt.Errorf("anirena monitoring requires an api key")
}

func (s *AnirenaScraper) updateTokenFromHeader(resp *http.Header) {
	if resp == nil {
		return
	}
	newToken := resp.Get("X-New-Token")
	if newToken != "" {
		s.mu.Lock()
		s.token = newToken
		// Extend expiry by 1 hour since X-New-Token resets token lifetime
		s.tokenExpiry = time.Now().Add(3600*time.Second - 30*time.Second)
		s.mu.Unlock()
	}
}

func (s *AnirenaScraper) FetchTorrents(
	uploader string,
	group string,
	keyword string,
	page int,
	sort string,
	order string,
) ([]AnirenaTorrent, int, error) {
	token, err := s.GetToken()
	if err != nil {
		return nil, 0, err
	}

	endpoint := fmt.Sprintf("%s/api/v1/torrents/search", s.baseURL)

	// Map search operators or unified params
	query := keyword
	if uploader != "" {
		if query != "" {
			query = fmt.Sprintf("user:%q %s", uploader, query)
		} else {
			query = fmt.Sprintf("user:%q", uploader)
		}
	}
	if group != "" {
		if query != "" {
			query = fmt.Sprintf("group:%q %s", group, query)
		} else {
			query = fmt.Sprintf("group:%q", group)
		}
	}

	searchBody := map[string]any{
		"q":        query,
		"page":     page,
		"per_page": 25,
	}
	if sort != "" {
		searchBody["sort"] = sort
	}
	if order != "" {
		searchBody["order"] = order
	}

	bodyBytes, err := json.Marshal(searchBody)
	if err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	s.updateTokenFromHeader(&resp.Header)

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("torrents/search returned HTTP status %d", resp.StatusCode)
	}

	var result AnirenaSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, err
	}

	for i := range result.Torrents {
		result.Torrents[i].Unescape()
	}

	return result.Torrents, result.TotalPages, nil
}

func (s *AnirenaScraper) FetchComments(torrentID string) ([]AnirenaComment, error) {
	token, err := s.GetToken()
	if err != nil {
		return nil, err
	}

	// Loop pages to get ALL comments (since we want all comments to process them)
	var allComments []AnirenaComment
	page := 1
	for {
		endpoint := fmt.Sprintf(
			"%s/api/v1/torrents/%s/comments?page=%d",
			s.baseURL,
			url.PathEscape(torrentID),
			page,
		)
		req, err := http.NewRequest("GET", endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

		resp, err := s.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		s.updateTokenFromHeader(&resp.Header)

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("torrents/comments returned HTTP status %d", resp.StatusCode)
		}

		var result AnirenaCommentsResult
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}

		allComments = append(allComments, result.Comments...)
		if page >= result.TotalPages || len(result.Comments) == 0 {
			break
		}
		page++
		time.Sleep(200 * time.Millisecond)
	}

	for i := range allComments {
		allComments[i].Unescape()
	}

	return allComments, nil
}
