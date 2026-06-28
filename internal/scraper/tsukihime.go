package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type TsukihimeScraper struct {
	baseURL string
	client  *http.Client
}

func NewTsukihimeScraper() *TsukihimeScraper {
	return &TsukihimeScraper{
		baseURL: "https://api.tsukihime.org",
		client:  NewHTTPClient(30 * time.Second),
	}
}

func (s *TsukihimeScraper) doRequest(req *http.Request) ([]byte, error) {
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
	ID          any    `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarHash  string `json:"avatar_hash"`
}

type TsukihimeComment struct {
	IDRaw       any              `json:"id"`
	UserIDRaw   any              `json:"user_id"`
	Content     string           `json:"content"`
	Text        string           `json:"text"`
	User        string           `json:"user"`
	Author      *TsukihimeAuthor `json:"author"`
	CreatedAt   string           `json:"created_at"`
	TargetIDRaw any              `json:"target_id"`
	TargetType  string           `json:"target_type"`
	ParentIDRaw any              `json:"parent_id"`
	ParentText  string           `json:"-"`
}

func (c *TsukihimeComment) GetUserID() string {
	return GetStringOrInt(c.UserIDRaw)
}

func GetStringOrInt(val any) string {
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

func (c *TsukihimeComment) GetTimestamp() int64 {
	if parsedTime, err := time.Parse(time.RFC3339, c.CreatedAt); err == nil {
		return parsedTime.Unix()
	} else if parsedTime, err := time.ParseInLocation("2006-01-02T15:04:05", c.CreatedAt, time.UTC); err == nil {
		return parsedTime.Unix()
	} else if parsedTime, err := time.ParseInLocation(time.DateTime, c.CreatedAt, time.UTC); err == nil {
		return parsedTime.Unix()
	}
	return time.Now().Unix()
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
	IsAdult      FlexBool               `json:"is_adult"`
	HasNZB       FlexBool               `json:"has_nzb"`
}

type TsukihimeTorrentDetails struct {
	ID        int                    `json:"id"`
	Name      string                 `json:"name"`
	AddedDate int64                  `json:"added_date"`
	Anime     *TsukihimeTorrentAnime `json:"anime"`
	Group     *TsukihimeTorrentGroup `json:"group"`
	IsAdult   FlexBool               `json:"is_adult"`
	HasNZB    FlexBool               `json:"has_nzb"`
}

type TsukihimeTorrentAnime struct {
	ID           any    `json:"id"`
	Title        string `json:"title"`
	EnglishTitle string `json:"english_title"`
	MAL          any    `json:"mal"`
	Anilist      any    `json:"anilist"`
	AniDB        any    `json:"anidb"`
}

type FlexBool bool

func (tb *FlexBool) UnmarshalJSON(data []byte) error {
	var val any
	if err := json.Unmarshal(data, &val); err != nil {
		return err
	}
	switch v := val.(type) {
	case bool:
		*tb = FlexBool(v)
	case float64:
		*tb = FlexBool(v != 0)
	case int:
		*tb = FlexBool(v != 0)
	case string:
		str := strings.ToLower(v)
		*tb = FlexBool(str == "true" || str == "1" || str == "yes")
	default:
		*tb = false
	}
	return nil
}

type TsukihimeTorrentGroup struct {
	ID       any      `json:"id"`
	Name     string   `json:"name"`
	IsFansub FlexBool `json:"is_fansub"`
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

	for i := range resp.Comments {
		resp.Comments[i].Unescape()
	}

	return &resp, nil
}

func (s *TsukihimeScraper) FetchTorrentsByAnime(
	animeID string,
	limit int,
	offset int,
) ([]TsukihimeTorrent, error) {
	u := fmt.Sprintf("%s/v1/animes/%s?limit=%d&offset=%d", s.baseURL, animeID, limit, offset)
	req, _ := http.NewRequest("GET", u, nil)

	body, err := s.doRequest(req)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Results []TsukihimeTorrent     `json:"results"`
		Anime   *TsukihimeTorrentAnime `json:"anime"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	for i := range resp.Results {
		if resp.Results[i].Anime == nil && resp.Anime != nil {
			resp.Results[i].Anime = resp.Anime
		}
		resp.Results[i].Unescape()
	}

	return resp.Results, nil
}

func (s *TsukihimeScraper) FetchTorrentsByGroup(
	groupID string,
	limit int,
	offset int,
) ([]TsukihimeTorrent, error) {
	u := fmt.Sprintf("%s/v1/groups/%s?limit=%d&offset=%d", s.baseURL, groupID, limit, offset)
	req, _ := http.NewRequest("GET", u, nil)

	body, err := s.doRequest(req)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Results []TsukihimeTorrent     `json:"results"`
		Group   *TsukihimeTorrentGroup `json:"group"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	for i := range resp.Results {
		if resp.Results[i].Group == nil && resp.Group != nil {
			resp.Results[i].Group = resp.Group
		}
		resp.Results[i].Unescape()
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
		ID any `json:"id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}

	return GetStringOrInt(resp.ID), nil
}

func (s *TsukihimeScraper) SearchTorrents(
	query string,
	limit int,
	offset int,
) ([]TsukihimeTorrent, error) {
	u := fmt.Sprintf(
		"%s/v1/search/torrents?q=%s&limit=%d&offset=%d",
		s.baseURL,
		url.QueryEscape(query),
		limit,
		offset,
	)
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

	for i := range resp.Results {
		resp.Results[i].Unescape()
	}

	return resp.Results, nil
}

func (s *TsukihimeScraper) FetchComments(
	torrentID string,
	title string,
) ([]TsukihimeComment, error) {
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

	details.Unescape()

	return &details, nil
}

func (s *TsukihimeScraper) FetchTorrents(limit int, offset int) ([]TsukihimeTorrent, error) {
	u := fmt.Sprintf("%s/v1/torrents?limit=%d&offset=%d", s.baseURL, limit, offset)
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

	for i := range resp.Results {
		resp.Results[i].Unescape()
	}

	return resp.Results, nil
}

func (s *TsukihimeScraper) ResolveGroupID(name string) (string, error) {
	limit := 50
	offset := 0
	for {
		u := fmt.Sprintf("%s/v1/groups?limit=%d&offset=%d", s.baseURL, limit, offset)
		req, _ := http.NewRequest("GET", u, nil)

		body, err := s.doRequest(req)
		if err != nil {
			return "", err
		}

		var resp struct {
			Total   int                     `json:"total"`
			Results []TsukihimeTorrentGroup `json:"results"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return "", err
		}

		if len(resp.Results) == 0 {
			break
		}

		for _, g := range resp.Results {
			if strings.EqualFold(g.Name, name) {
				return GetStringOrInt(g.ID), nil
			}
		}

		offset += limit
		if offset >= resp.Total {
			break
		}
	}
	return "", fmt.Errorf("group name %q not found", name)
}
