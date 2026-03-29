package main

import (
	"context"
	"database/sql"
	"fmt"
	"go_freecodecamp/internal/database"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

func startScraping(
	db *database.Queries,
	concurrency int,
	timeBetweenRequest time.Duration,
) {
	log.Printf("Scraping on %v goroutines every %s duration", concurrency, timeBetweenRequest)
	ticker := time.NewTicker(timeBetweenRequest)
	for ; ; <-ticker.C {
		feeds, err := db.GetNextFeedsToFetch(
			context.Background(),
			int32(concurrency),
		)
		if err != nil {
			log.Printf("Error getting feeds to fetch: %v", err)
			continue
		}
		wg := &sync.WaitGroup{}
		for _, feed := range feeds {
			wg.Add(1)

			go scrapeFeed(db, wg, feed)
		}
		wg.Wait()
	}
}

func scrapeFeed(db *database.Queries, wg *sync.WaitGroup, feed database.Feed) {
	defer wg.Done()

	_, err := db.MarkFeedAsFetched(context.Background(), feed.ID)
	if err != nil {
		log.Printf("Error marking feed as fetched %v: %v", feed.ID, err)
	}

	rssFeed, err := urlToFeed(feed.Url)
	if err != nil {
		log.Printf("Error fetching RSS feed %v: %v", feed.ID, err)
		return
	}

	for _, item := range rssFeed.Channel.Item {

		description := sql.NullString{}
		if item.Description != "" {
			description.String = item.Description
			description.Valid = true
		}

		pubAt, err := parsePublishedAt(item.PubDate)
		if err != nil {
			log.Printf("Error parsing publication date %v: %v", item.PubDate, err)
			continue
		}

		_, err = db.CreatePost(context.Background(),
			database.CreatePostParams{
				ID:          uuid.New(),
				UpdatedAt:   time.Now().UTC(),
				CreatedAt:   time.Now().UTC(),
				Title:       item.Title,
				Description: description,
				PublishedAt: pubAt.UTC(),
				Url:         item.Link,
				FeedID:      feed.ID,
			})

		if err != nil {
			if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
				continue
			}
			log.Printf("Error creating post for feed %v: %v", feed.ID, err)
			continue
		}
	}

	log.Printf("Feed %s collected, %v posts found", feed.Name, len(rssFeed.Channel.Item))
}

func parsePublishedAt(pubDate string) (time.Time, error) {
	pubDate = strings.TrimSpace(pubDate)

	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		"Mon, _2 Jan 2006 15:04:05 -0700",
		"Mon, _2 Jan 2006 15:04:05 MST",
		time.RFC822Z,
		time.RFC822,
		time.RFC3339,
	}

	var lastErr error
	for _, layout := range layouts {
		parsedTime, err := time.Parse(layout, pubDate)
		if err == nil {
			return parsedTime, nil
		}
		lastErr = err
	}

	return time.Time{}, fmt.Errorf("unsupported pubDate format %q: %w", pubDate, lastErr)
}
