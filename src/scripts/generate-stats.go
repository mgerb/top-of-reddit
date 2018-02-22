package main

import (
	"encoding/json"
	"log"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/mgerb/top-of-reddit/src/model"
	"github.com/olekukonko/tablewriter"

	"github.com/boltdb/bolt"
)

var conn *bolt.DB

func init() {
	conn, _ = bolt.Open("../reddit.db", 0600, &bolt.Options{Timeout: 1 * time.Second})
}

func main() {

	posts, err := getAllPosts()

	if err != nil {
		log.Fatal(err)
	}

	err = writeSubredditListToFile(posts)

	if err != nil {
		log.Fatal(err)
	}

	err = writeStatsToFile(posts)

	if err != nil {
		log.Fatal(err)
	}

}

// get posts from database file
func getAllPosts() ([]model.RedditPost, error) {
	posts := []model.RedditPost{}

	err := conn.View(func(tx *bolt.Tx) error {
		dailyBucket := tx.Bucket([]byte("daily_bucket"))

		return dailyBucket.ForEach(func(key, val []byte) error {

			b := dailyBucket.Bucket(key)

			return b.ForEach(func(k, v []byte) error {
				var post model.RedditPost
				err := json.Unmarshal(b.Get(k), &post)
				if err != nil {
					return err
				}
				posts = append(posts, post)
				return nil
			})
		})

	})

	return posts, err
}

// write subreddits to file for the word cloud generator
func writeSubredditListToFile(posts []model.RedditPost) error {
	for _, post := range posts {
		err := appendFile("subreddits.txt", post.Subreddit)
		if err != nil {
			return err
		}
	}
	return nil
}

// create markdown table with subreddit stats
func writeStatsToFile(posts []model.RedditPost) error {

	groupedPosts := groupBySubreddit(posts)

	countList := [][]model.RedditPost{}

	// convert to list
	for _, v := range groupedPosts {
		countList = append(countList, v)
	}

	// sort by post count
	sort.Slice(countList, func(i, j int) bool {
		return len(countList[i]) > len(countList[j])
	})

	data := [][]string{}

	for _, v := range countList {
		title := "[" + v[0].Title + "]" + "(https://www.reddit.com" + v[0].Permalink + ")"
		data = append(data, []string{v[0].Subreddit, strconv.Itoa(len(v)), title, strconv.Itoa(v[0].Score)})
	}

	file, _ := os.Create("counts.md")

	table := tablewriter.NewWriter(file)
	table.SetAutoWrapText(false)
	table.SetHeader([]string{"Subreddit", "Total", "Top Post", "Score"})
	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetCenterSeparator("|")
	table.AppendBulk(data) // Add Bulk Data
	table.Render()

	return nil
}

func groupBySubreddit(posts []model.RedditPost) map[string][]model.RedditPost {

	groupedPosts := map[string][]model.RedditPost{}

	// group posts by subreddit
	for _, v := range posts {
		if _, ok := groupedPosts[v.Subreddit]; ok {
			groupedPosts[v.Subreddit] = append(groupedPosts[v.Subreddit], v)
		} else {
			groupedPosts[v.Subreddit] = []model.RedditPost{v}
		}
	}

	// order posts by view count
	for _, v := range groupedPosts {
		sort.Slice(v, func(i, j int) bool {
			return v[i].Score > v[j].Score
		})
	}

	return groupedPosts
}

func appendFile(path, text string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(text + "\n")
	if err != nil {
		return err
	}
	return nil
}
