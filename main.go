package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sync"
	"time"
)

// Store ...
type Store struct {
	mu    sync.RWMutex
	ids   []int
	items []Item
}

// Item ...
type Item struct {
	Index int    `json:"-"`
	Title string `json:"title"`
	Text  string `json:"text"`
	URL   string `json:"url"`
	Host  string `json:"-"`
}

const numberOfItemsToDisplay int = 30
const apiURL string = "https://hacker-news.firebaseio.com/v0/"
const refreshInterval = (1 * time.Hour)

var (
	env  = os.Getenv("ENV")
	port = os.Getenv("PORT")

	indexTmpl = template.Must(template.ParseFiles("index.tmpl"))
	store     = newStore()
)

func main() {
	if port == "" {
		panic("ERROR: No port set")
	}

	go store.getTopStories() // Initial population of stories

	mux := http.NewServeMux()

	fs := http.FileServer(http.Dir("static"))
  mux.Handle("/static/", http.StripPrefix("/static/", fs))

	mux.HandleFunc("/", redirectToHTTPS(index))

	hs := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	ticker := time.NewTicker(refreshInterval)
	quit := make(chan struct{})

	go func() {
		for {
			select {
			case <-ticker.C:
				store.getTopStories()

			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	err := hs.ListenAndServe()
	if err != nil {
		log.Fatal("Listen and Serve: ", err)
	}
}

func index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	if err := indexTmpl.Execute(w, store.Items()); err != nil {
		log.Fatal(err)
	}
}

func redirectToHTTPS(fn func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if env != "dev" { // Avoid doing this in development means we do not have to set up local certificates.
			if r.Header.Get("X-Forwarded-Proto") != "https" { // Heroku sends this header to identify HTTPS/HTTP
				target := "https://" + r.Host + r.URL.Path

				http.Redirect(w, r, target, http.StatusMovedPermanently)
			}
		}

		fn(w, r)
	}
}

func (s *Store) getTopStories() {
	s.fetchIDs()
	s.fetchItems(s.IDs())
}

func (s *Store) fetchIDs() {
	r, err := http.Get(apiURL + "topstories.json")
	if err != nil {
		log.Println("ERROR: Unable to access Hacker News API")
		log.Println(err)
	}
	defer r.Body.Close()

	var ids []int

	dec := json.NewDecoder(r.Body)
	err = dec.Decode(&ids)

	if err != nil {
		log.Println(err)
	}

	s.SetIDs(ids)
}

func (s *Store) fetchItems(ids []int) {
	var items []Item // Temporary storage to switch out with real storage
	var numItems int

	for _, id := range ids {
		itemURL := fmt.Sprintf("%sitem/%d.json", apiURL, id)

		item, err := fetchItem(itemURL)

		// We encountered an error, or it's a discussion link
		if err != nil || item.URL == "" || item.Text != "" {
			continue
		}

		items = append(items, *item)

		numItems++

		if numItems >= numberOfItemsToDisplay {
			break
		}
	}

	s.SetItems(items)
}

func fetchItem(apiURL string) (*Item, error) {
	var i Item

	r, err := http.Get(apiURL)
	if err != nil {
		log.Printf("ERROR: Get Request | %v\n", apiURL)
		log.Println(err)
		return nil, err
	}
	defer r.Body.Close()

	if r.StatusCode != http.StatusOK {
		return nil, errors.New("Did not receive 200 OK from HN API")
	}

	dec := json.NewDecoder(r.Body)
	err = dec.Decode(&i)

	if err != nil {
		log.Println(err)
		return nil, err
	}

	u, err := url.Parse(i.URL)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	i.Host = trimSubdomain(u.Hostname())

	return &i, nil
}

func newStore() *Store {
	return &Store{items: []Item{}}
}

func (s *Store) SetIDs(ids []int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ids = ids
}

func (s *Store) IDs() []int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.ids
}

func (s *Store) SetItems(items []Item) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.items = items
}

func (s *Store) Items() []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.items
}

func trimSubdomain(s string) string {
	re, err := regexp.Compile(`(https?://|w{3}\.)`)
	if err != nil {
		log.Println(err)
		return ""
	}

	return re.ReplaceAllString(s, "")
}
