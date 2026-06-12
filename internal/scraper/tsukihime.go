package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type TsukihimeScraper struct {
	baseURL string
	client  *http.Client
}

func NewTsukihimeScraper() *TsukihimeScraper {
	return &TsukihimeScraper{
		baseURL: "https://api.tsukihime.org",
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *TsukihimeScraper) doRequest(req *http.Request) ([]byte, error) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

type TsukihimeAuthor struct {
	ID          interface{} `json:"id"`
	Username    string      `json:"username"`
	DisplayName string      `json:"display_name"`
	AvatarHash  string      `json:"avatar_hash"`
}

type TsukihimeComment struct {
	IDRaw       interface{}      `json:"id"`
	UserIDRaw   interface{}      `json:"user_id"`
	Content     string           `json:"content"`
	Text        string           `json:"text"`
	User        string           `json:"user"`
	Author      *TsukihimeAuthor `json:"author"`
	CreatedAt   string           `json:"created_at"`
	TargetIDRaw interface{}      `json:"target_id"`
	TargetType  string           `json:"target_type"`
	ParentIDRaw interface{}      `json:"parent_id"`
	ParentText  string           `json:"-"`
}

func (c *TsukihimeComment) GetUserID() string {
	return GetStringOrInt(c.UserIDRaw)
}

func GetStringOrInt(val interface{}) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	}
	return fmt.Sprintf("%v", val)
}

func (c *TsukihimeComment) GetID() string {
	return GetStringOrInt(c.IDRaw)
}

func (c *TsukihimeComment) GetTargetID() string {
	return GetStringOrInt(c.TargetIDRaw)
}

func (c *TsukihimeComment) GetParentID() string {
	return GetStringOrInt(c.ParentIDRaw)
}

func (c *TsukihimeComment) GetUsername() string {
	if c.Author != nil && c.Author.Username != "" {
		return c.Author.Username
	}
	return c.User
}

func (c *TsukihimeComment) GetDisplayName() string {
	if c.Author != nil && c.Author.DisplayName != "" {
		return c.Author.DisplayName
	}
	if c.Author != nil && c.Author.Username != "" {
		return c.Author.Username
	}
	return c.User
}

func (c *TsukihimeComment) GetText() string {
	if c.Text != "" {
		return c.Text
	}
	return c.Content
}

type TsukihimeResponse struct {
	Total    int                `json:"total"`
	Start    int                `json:"start"`
	Limit    int                `json:"limit"`
	Error    bool               `json:"error"`
	Comments []TsukihimeComment `json:"comments"`
}

type TsukihimeTorrent struct {
	ID           int                    `json:"id"`
	Name         string                 `json:"name"`
	BTIH         string                 `json:"btih"`
	Totalsize    int64                  `json:"totalsize"`
	AddedDate    int64                  `json:"added_date"`
	CommentCount int                    `json:"comment_count"`
	Anime        *TsukihimeTorrentAnime `json:"anime"`
	Group        *TsukihimeTorrentGroup `json:"group"`
}

type TsukihimeTorrentDetails struct {
	ID    int                    `json:"id"`
	Name  string                 `json:"name"`
	Anime *TsukihimeTorrentAnime `json:"anime"`
	Group *TsukihimeTorrentGroup `json:"group"`
}

type TsukihimeTorrentAnime struct {
	ID           interface{} `json:"id"`
	Title        string      `json:"title"`
	EnglishTitle string      `json:"english_title"`
	MAL          interface{} `json:"mal"`
	Anilist      interface{} `json:"anilist"`
	AniDB        interface{} `json:"anidb"`
}

type TsukihimeTorrentGroup struct {
	ID   interface{} `json:"id"`
	Name string      `json:"name"`
}

func (s *TsukihimeScraper) FetchLatestComments(limit int, offset int) (*TsukihimeResponse, error) {
	u := fmt.Sprintf("%s/comments/latest?limit=%d&offset=%d", s.baseURL, limit, offset)
	req, _ := http.NewRequest("GET", u, nil)

	body, err := s.doRequest(req)
	if err != nil {
		return nil, err
	}

	var resp TsukihimeResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (s *TsukihimeScraper) FetchTorrentsByAnime(animeID string) ([]TsukihimeTorrent, error) {
	u := fmt.Sprintf("%s/v1/animes/%s?limit=100", s.baseURL, animeID)
	req, _ := http.NewRequest("GET", u, nil)

	body, err := s.doRequest(req)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Results []TsukihimeTorrent `json:"results"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	return resp.Results, nil
}

func (s *TsukihimeScraper) FetchTorrentsByGroup(groupID string) ([]TsukihimeTorrent, error) {
	u := fmt.Sprintf("%s/v1/groups/%s?limit=100", s.baseURL, groupID)
	req, _ := http.NewRequest("GET", u, nil)

	body, err := s.doRequest(req)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Results []TsukihimeTorrent `json:"results"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	return resp.Results, nil
}

func (s *TsukihimeScraper) ResolveAnimeID(service, id string) (string, error) {
	u := fmt.Sprintf("%s/v1/animes/%s/%s", s.baseURL, service, id)
	req, _ := http.NewRequest("GET", u, nil)

	body, err := s.doRequest(req)
	if err != nil {
		return "", err
	}

	var resp struct {
		ID interface{} `json:"id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}

	return GetStringOrInt(resp.ID), nil
}

func (s *TsukihimeScraper) SearchTorrents(query string) ([]TsukihimeTorrent, error) {
	u := fmt.Sprintf("%s/v1/search/torrents?q=%s&limit=100", s.baseURL, url.QueryEscape(query))
	req, _ := http.NewRequest("GET", u, nil)

	body, err := s.doRequest(req)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Results []TsukihimeTorrent `json:"results"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	return resp.Results, nil
}

func (s *TsukihimeScraper) FetchComments(torrentID string, title string) ([]TsukihimeComment, error) {
	resp, err := s.FetchLatestComments(100, 0)
	if err != nil {
		return nil, err
	}

	var comments []TsukihimeComment
	commentMap := make(map[string]TsukihimeComment)
	for _, c := range resp.Comments {
		commentMap[c.GetID()] = c
	}

	for _, c := range resp.Comments {
		if c.TargetType == "torrent" && c.GetTargetID() == torrentID {
			parentID := c.GetParentID()
			if parentID != "" {
				if parent, ok := commentMap[parentID]; ok {
					c.ParentText = parent.GetText()
				}
			}
			comments = append(comments, c)
		}
	}

	return comments, nil
}

func (s *TsukihimeScraper) FetchTorrentDetails(torrentID string) (*TsukihimeTorrentDetails, error) {
	u := fmt.Sprintf("%s/v1/torrents/%s", s.baseURL, torrentID)
	req, _ := http.NewRequest("GET", u, nil)

	body, err := s.doRequest(req)
	if err != nil {
		return nil, err
	}

	var details TsukihimeTorrentDetails
	if err := json.Unmarshal(body, &details); err != nil {
		return nil, err
	}

	return &details, nil
}
