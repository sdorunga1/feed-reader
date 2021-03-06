package feedlist

import (
	"encoding/json"
	"errors"
	"log"

	"github.com/boltdb/bolt"
	"github.com/google/uuid"
)

const (
	feedListBucket    = "feedlist"
	allFeedsBucketKey = "all"
)

var (
	// Note: I only included this to make the app easier to demo, in the
	// real version this would just not be here and we'd have to set up
	// every entry with a call to the POST /feeed handler
	defaultFeedsList = []Feed{
		Feed{
			ID:          "b1031651-411c-40bb-b269-d247794dfd59",
			Title:       "BBC News - UK",
			Description: "BBC News - UK",
			URL:         "http://feeds.bbci.co.uk/news/uk/rss.xml",
			ImageURL:    "https://news.bbcimg.co.uk/nol/shared/img/bbc_news_120x60.gif",
		},
		Feed{
			ID:          "c2970c84-37c8-4ec1-8861-4b5a91ebff0d",
			Title:       "BBC News - Technology",
			Description: "BBC News - Technology",
			URL:         "http://feeds.bbci.co.uk/news/technology/rss.xml",
			ImageURL:    "https://news.bbcimg.co.uk/nol/shared/img/bbc_news_120x60.gif",
		},
		Feed{
			ID:          "28059396-5113-46ed-b76b-6d482a3bbcf3",
			Title:       "UK News - The latest headlines from the UK | Sky News",
			Description: "Expert comment and analysis on the latest UK news, with headlines from England, Scotland, Northern Ireland and Wales.",
			URL:         "http://feeds.skynews.com/feeds/rss/uk.xml",
			Category:    "Sky News",
			ImageURL:    "http://feeds.skynews.com/images/web/logo/skynews_rss.png",
		},
		Feed{
			ID:          "a2370e4f-0e7f-4844-83cb-b54c02b0bf1f",
			Title:       "Tech News - Latest Technology and Gadget News | Sky News",
			Description: "Sky News technology provides you with all the latest tech and gadget news, game reviews, Internet and web news across the globe. Visit us today.",
			URL:         "http://feeds.skynews.com/feeds/rss/technology.xml",
			Category:    "Sky News",
			ImageURL:    "http://feeds.skynews.com/images/web/logo/skynews_rss.png",
		},
	}
	// ErrorInitializingDB occurs only on startup when we are trying to get the
	// DB set up
	ErrorInitializingDB = errors.New("Error Initializing DB")
	// ErrorUnconfiguredBucket occurs when there is a mismatch between the
	// bucket we are using to read and what exists in the DB
	ErrorUnconfiguredBucket = errors.New("Error Unconfigured Bucket")
	// ErrorStoreFeedListCorrupted occurs if somehow we store an invalid
	// JSON blob is stored in the db
	ErrorStoreFeedListCorrupted = errors.New("Corrupted stored field list")
	// ErrorFeedNotFound occurs if somehow we store an invalid
	// JSON blob is stored in the db
	ErrorFeedNotFound = errors.New("Feed does not exist")
)

// FeedListStore is where we keep the available feeds that can be queried to
// get articles
type FeedListStore struct {
	db *bolt.DB
}

// Feed The stored version of a Feed, only includes what we need for the listing
// endpoint and for querying
type Feed struct {
	ID          string
	Title       string
	Description string
	URL         string
	ImageURL    string
	Category    string
}

// NewFeedListStore returns a fully initialised FeedListStore and should be the
// only way used to get a hold of one
//
// Note: Should have a var storing a Singleton FeedListStore that I mutex lock
// around to ensure I only have bolt DB open at any one time. I'm ignoring this
// now because it's very similar to the CachedFetcher example
func NewFeedListStore(db *bolt.DB) (FeedListStore, error) {
	store := FeedListStore{
		db: db,
	}
	if err := store.init(); err != nil {
		return FeedListStore{}, err
	}

	return store, nil
}

// ListAll returns a list of all the Feeds that have been stored, including a
// hardcoded list for demo purposes
func (store FeedListStore) ListAll() ([]Feed, error) {
	storedFeedList, err := store.listStored()
	if err != nil {
		return []Feed{}, err
	}

	if storedFeedList == nil {
		return defaultFeedsList, nil
	}

	return append(defaultFeedsList, storedFeedList...), nil
}

func (store FeedListStore) listStored() ([]Feed, error) {
	rawFeedList := []byte{}
	err := store.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(feedListBucket))
		if bucket == nil {
			log.Printf("Error: Bucket `" + feedListBucket + "` is unconfigured")
			return ErrorUnconfiguredBucket
		}
		rawFeedList = bucket.Get([]byte(allFeedsBucketKey))
		return nil
	})

	if err != nil {
		return []Feed{}, err
	}

	if len(rawFeedList) == 0 {
		return []Feed{}, nil
	}

	feedList := []Feed{}
	err = json.Unmarshal(rawFeedList, &feedList)
	if err != nil {
		log.Printf("Error: can't unmarshal feed %v", rawFeedList)
		return []Feed{}, ErrorStoreFeedListCorrupted
	}

	return feedList, nil
}

// Add inserts a new Feed into the list, it also ensures we don't add a feed
// twice
//
// Note: Because of the sequence in this func there is a race condition
// where we could add the same item to the DB twice if two requests come in
// simultaneously. I'm ignoring it as it is again the same solution as the
// Cached Fetcher, it's also not usually an issue with conventional databases
// as we could have some uniqueness constraint set up
func (store FeedListStore) Add(feed Feed) (string, error) {
	feed.ID = uuid.NewString()
	existingFeeds, err := store.listStored()
	if err != nil {
		return "", err
	}

	for _, existingFeed := range existingFeeds {
		// Don't add an existing feed to the DB
		if existingFeed.URL == feed.URL {
			return existingFeed.ID, nil
		}
	}

	// Note: this wouldn't be needed in the final version as everything would
	// be DB driven so we'd only range over the actual stored Feeds
	for _, existingFeed := range defaultFeedsList {
		// Don't add an existing feed to the DB
		if existingFeed.URL == feed.URL {
			return existingFeed.ID, nil
		}
	}

	rawFeed, err := json.Marshal(append(existingFeeds, feed))
	if err != nil {
		log.Printf("Error: Failed to marshal feed %v", feed)
		return "", err
	}
	err = store.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(feedListBucket))
		if bucket == nil {
			log.Printf("Error: Unconfigured bucket")
			return ErrorUnconfiguredBucket
		}
		err := bucket.Put([]byte(allFeedsBucketKey), rawFeed)
		return err
	})

	if err != nil {
		return "", err
	}

	return feed.ID, nil
}

// GetByID returns one stored feed by the matching ID field
func (store FeedListStore) GetByID(ID string) (Feed, error) {
	feeds, err := store.ListAll()
	if err != nil {
		return Feed{}, err
	}

	for _, feed := range feeds {
		// Don't add an existing feed to the DB
		if feed.ID == ID {
			return feed, nil
		}
	}

	return Feed{}, ErrorFeedNotFound
}

func (store FeedListStore) init() error {
	return store.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(feedListBucket))
		if err != nil {
			log.Printf("Error creating bucket: %s", err)
			return ErrorInitializingDB
		}
		return nil
	})
}
