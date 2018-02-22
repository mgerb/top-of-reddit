package model

// RedditPost -
type RedditPost struct {
	Subreddit    string  `json:"subreddit"`
	ID           string  `json:"id"`
	Gilded       int     `json:"gilded"`
	Score        int     `json:"score"`
	Author       string  `json:"author"`
	Domain       string  `json:"domain"`
	Over_18      bool    `json:"over_18"`
	Thumbnail    string  `json:"thumbnail"`
	Permalink    string  `json:"permalink"`
	Url          string  `json:"url"`
	Title        string  `json:"title"`
	Created      float64 `json:"created"`
	Created_utc  float64 `json:"created_utc"`
	Num_comments int     `json:"num_comments"`
	Ups          int     `json:"ups"`

	// extra fields
	TopPosition int // highest achieved position on front page
}
