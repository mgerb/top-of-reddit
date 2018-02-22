package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"time"

	"github.com/boltdb/bolt"
	"github.com/mgerb/top-of-reddit/src/model"
	"github.com/tidwall/gjson"
)

const (
	REDDIT_URL   string = "https://www.reddit.com/r/"
	USER_AGENT   string = "top-of-reddit:bot"
	DATE_FORMAT  string = "01-02-2006"
	YEAR_FORMAT  string = "2006"
	MONTH_FORMAT string = "01"
)

var (
	// buckets
	DAILY_BUCKET []byte = []byte("daily_bucket")
	MAIN_BUCKET  []byte = []byte("main")

	// store the current day to keep track when day turns over
	TODAY_KEY []byte = []byte("today_date")
)

func main() {
	// start database connection
	db := openDbSession()
	defer db.Close()

	for {
		fmt.Println("Updating...")
		// get reddit posts from r/all
		response, err := getPosts("all")
		if err != nil {
			log.Println(err.Error())
		} else {
			// store posts in RedditPost slice
			posts, err := convertPosts(response)
			if err != nil {
				log.Println(err.Error())
			} else {
				// update the daily bucket with posts
				updateDailyPosts(db, DAILY_BUCKET, getTodayBucket(), posts)
				checkDateChange(db)
			}

		}

		time.Sleep(time.Second * 30)
	}
}

// open database session
func openDbSession() *bolt.DB {
	database, err := bolt.Open("reddit.db", 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		log.Fatal(err)
	}

	return database
}

// returns the post bucket for today
func getTodayBucket() []byte {
	return []byte(time.Now().Format(DATE_FORMAT))
}

// get time object of yesterday
func getYesterdayTime() time.Time {
	return time.Now().AddDate(0, 0, -1)
}

// returns the post bucket for yesterday
func getYesterdayBucket() []byte {
	return []byte(getYesterdayTime().Format(DATE_FORMAT))
}

// returns date string for folder path
func getFolderPath() string {
	yesterday := getYesterdayTime()
	year := yesterday.Format(YEAR_FORMAT)
	month := yesterday.Format(MONTH_FORMAT)
	return "../" + year + "/" + month
}

func checkDateChange(db *bolt.DB) {
	err := db.Update(func(tx *bolt.Tx) error {

		b, err := tx.CreateBucketIfNotExists(MAIN_BUCKET)

		if err != nil {
			return err
		}

		storedDay := b.Get(TODAY_KEY)

		// if day turns over
		if storedDay == nil || string(getTodayBucket()) != string(storedDay) {
			// set today's date in database
			err := b.Put(TODAY_KEY, getTodayBucket())

			if err != nil {
				return err
			}

			// if no data exists for yesterday
			if storedDay == nil {
				storedDay = getTodayBucket()
			}

			fmt.Println("Creating markdown!")

			storedPosts, err := getStoredPosts(db, DAILY_BUCKET, storedDay)

			if err != nil {
				return err
			}

			err = writePostsToFile(string(storedDay), storedPosts)

			if err != nil {
				return err
			}

			// push to github
			err = pushToGithub()

			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		log.Println(err)
		return
	}
}

func writePostsToFile(fileName string, posts []model.RedditPost) error {
	folderPath := getFolderPath()

	// create directory if not exists
	if _, err := os.Stat(folderPath); os.IsNotExist(err) {
		os.MkdirAll(folderPath, 0700)
	}

	// create new markdown file
	file, err := os.Create(folderPath + "/" + fileName + ".md")
	defer file.Close()

	if err != nil {
		return err
	}

	for index, p := range posts {
		permalink := "http://reddit.com" + p.Permalink
		file.WriteString("## " + strconv.Itoa(index+1) + ". [" + p.Title + "](" + permalink + ") - " + strconv.Itoa(p.Score) + "\n")
		file.WriteString("#### [r/" + p.Subreddit + "](http://reddit.com/r/" + p.Subreddit + ")")
		file.WriteString(" - [u/" + p.Author + "](http://reddit.com/u/" + p.Author + ") - ")
		file.WriteString(strconv.Itoa(p.Num_comments) + " Comments - ")
		file.WriteString("Top position achieved: " + strconv.Itoa(p.TopPosition) + "\n\n")

		// don't post image link if thumbnail doesn't exist
		if p.Thumbnail == "default" || p.Thumbnail == "self" {
			continue
		}

		// don't show thumbnail if NSFW
		if p.Over_18 {
			file.WriteString("<a href=\"" + p.Url + "\"><img src=\"https://github.com/mgerb/top-of-reddit/raw/master/nsfw.jpg\"></img></a>\n\n")
		} else {
			file.WriteString("<a href=\"" + p.Url + "\"><img src=\"" + p.Thumbnail + "\"></img></a>\n\n")
		}
	}

	file.Sync()

	return nil
}

// get a RedditPost slice
func getStoredPosts(db *bolt.DB, bucket []byte, day []byte) ([]model.RedditPost, error) {

	posts := []model.RedditPost{}

	err := db.View(func(tx *bolt.Tx) error {
		tx.Bucket(bucket).Bucket(day).ForEach(func(_, v []byte) error {
			tempPost := model.RedditPost{}
			err := json.Unmarshal(v, &tempPost)
			posts = append(posts, tempPost)

			if err != nil {
				return err
			}

			return nil
		})

		return nil
	})

	// sort posts by score
	sort.Sort(ByScore(posts))

	if err != nil {
		return []model.RedditPost{}, err
	}

	return posts, nil
}

// stores new posts in the bucket only if they do not exist
func updateDailyPosts(db *bolt.DB, bucket []byte, day []byte, redditPosts []model.RedditPost) error {
	err := db.Update(func(tx *bolt.Tx) error {

		daily_bucket, err := tx.CreateBucketIfNotExists(bucket)
		if err != nil {
			return err
		}

		today, err := daily_bucket.CreateBucketIfNotExists(day)
		if err != nil {
			return err
		}

		for index, post := range redditPosts {
			// check if post was in yesterdays top posts
			yesterday := daily_bucket.Bucket(getYesterdayBucket())
			if yesterday != nil && yesterday.Get([]byte(post.ID)) != nil {
				continue
			}

			post.TopPosition = index + 1

			// get value stored in database
			storedPostString := today.Get([]byte(post.ID))

			// if post is already stored in database - check to update highest score
			if storedPostString != nil {
				storedPost := model.RedditPost{}
				err := json.Unmarshal(storedPostString, &storedPost)
				if err != nil {
					return err
				}

				// only store the highest score a post achieves
				if storedPost.Score > post.Score {
					post.Score = storedPost.Score
				}

				// only store the highest position a post achieves
				if storedPost.TopPosition < index+1 {
					post.TopPosition = storedPost.TopPosition
				}
			} else {
				fmt.Println("Updating new post: " + post.Title)
			}

			// convert json to string
			postString, err := json.Marshal(post)
			if err != nil {
				return err
			}

			// store in database
			err = today.Put([]byte(post.ID), []byte(postString))
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

// convert reddit response string to RedditPost slice
func convertPosts(postString string) ([]model.RedditPost, error) {
	posts := []model.RedditPost{}

	for _, p := range gjson.Get(postString, "data.children").Array() {
		tempPost := model.RedditPost{}

		err := json.Unmarshal([]byte(p.Get("data").String()), &tempPost)
		if err != nil {
			return posts, err
		}

		posts = append(posts, tempPost)
	}

	return posts, nil
}

// send http request to reddit
func getPosts(subreddit string) (string, error) {
	client := &http.Client{}

	req, err := http.NewRequest("GET", REDDIT_URL+subreddit+".json", nil)

	req.Header.Add("User-Agent", USER_AGENT)

	response, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func pushToGithub() error {
	fmt.Println("Pushing to Github...")
	commitMessage := "Adding posts for " + string(getYesterdayBucket())

	out, err := exec.Command("git", "add", ".").Output()
	if err != nil {
		return err
	}
	fmt.Println(string(out))

	out, err = exec.Command("git", "commit", "-m", commitMessage).Output()
	if err != nil {
		return err
	}
	fmt.Println(string(out))

	out, err = exec.Command("git", "push", "origin", "master").Output()
	if err != nil {
		return err
	}
	fmt.Println(string(out))

	return nil
}

// sorting
type ByScore []model.RedditPost

func (s ByScore) Len() int {
	return len(s)
}

func (s ByScore) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s ByScore) Less(i, j int) bool {
	return s[i].Score > s[j].Score
}
